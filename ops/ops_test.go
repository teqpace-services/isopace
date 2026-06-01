// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction framework.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.

package ops_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teqpace-services/isopace/ops"
	"github.com/teqpace-services/isopace/rbac"
	"github.com/teqpace-services/isopace/runtime"
)

func TestHealth(t *testing.T) {
	h := ops.NewHealth()
	if r := h.Check(context.Background()); r.Status != ops.StatusHealthy {
		t.Errorf("empty health = %v want healthy", r.Status)
	}
	h.Register("db", func(context.Context) error { return nil })
	h.Register("queue", func(context.Context) error { return errors.New("down") })
	r := h.Check(context.Background())
	if r.Status != ops.StatusUnhealthy {
		t.Errorf("status = %v want unhealthy", r.Status)
	}
	if r.Checks["db"].OK != true || r.Checks["queue"].OK != false {
		t.Errorf("check results = %+v", r.Checks)
	}
	if r.Checks["queue"].Error != "down" {
		t.Errorf("error detail = %q", r.Checks["queue"].Error)
	}
	h.Deregister("queue")
	if r := h.Check(context.Background()); r.Status != ops.StatusHealthy {
		t.Errorf("after deregister = %v want healthy", r.Status)
	}
}

func TestMetricsExposition(t *testing.T) {
	var m runtime.Observer = ops.NewMetrics() // also satisfies the facade
	reg := m.(*ops.Metrics)

	m.Counter("tx_total").Add(2, runtime.A("mti", "0200"))
	m.Counter("tx_total").Add(3, runtime.A("mti", "0200"))
	reg.Gauge("open_conns").Set(5)
	m.Histogram("latency_seconds").Observe(0.25)
	m.Histogram("latency_seconds").Observe(0.75)

	var sb strings.Builder
	if err := reg.WriteText(&sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{
		"# TYPE tx_total counter",
		`tx_total{mti="0200"} 5`,
		"# TYPE open_conns gauge",
		"open_conns 5",
		"# TYPE latency_seconds summary",
		"latency_seconds_count 2",
		"latency_seconds_sum 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestStaticCluster(t *testing.T) {
	c := ops.NewStaticCluster(ops.Member{ID: "n2", Addr: "host2:5000"}, ops.Member{ID: "n1", Addr: "host1:5000"})
	if !c.Self().Self || c.Self().ID != "n2" {
		t.Errorf("Self = %+v", c.Self())
	}
	members := c.Members()
	if len(members) != 2 || members[0].ID != "n1" || members[1].ID != "n2" {
		t.Errorf("Members = %+v (want sorted n1,n2)", members)
	}
}

type stubIntrospector struct{ names []string }

func (s stubIntrospector) Components() []string { return s.names }

func TestAPIPublicEndpoints(t *testing.T) {
	h := ops.NewHealth()
	h.Register("ok", func(context.Context) error { return nil })
	m := ops.NewMetrics()
	m.Counter("c").Add(1)

	api := ops.NewAPI(
		ops.WithHealth(h),
		ops.WithMetrics(m),
		ops.WithIntrospector(stubIntrospector{[]string{"link", "mux"}}),
		ops.WithInfo(map[string]string{"version": "0.1.0"}),
	)
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	if body, code := get(t, srv.URL+"/healthz", ""); code != 200 || !strings.Contains(body, "ok") {
		t.Errorf("/healthz = %d %q", code, body)
	}
	if body, code := get(t, srv.URL+"/readyz", ""); code != 200 || !strings.Contains(body, "healthy") {
		t.Errorf("/readyz = %d %q", code, body)
	}
	if body, code := get(t, srv.URL+"/metrics", ""); code != 200 || !strings.Contains(body, "# TYPE c counter") {
		t.Errorf("/metrics = %d %q", code, body)
	}
	// No auth policy configured -> /info open.
	if body, code := get(t, srv.URL+"/info", ""); code != 200 || !strings.Contains(body, "0.1.0") || !strings.Contains(body, "link") {
		t.Errorf("/info = %d %q", code, body)
	}
}

func TestAPIReadinessUnhealthy(t *testing.T) {
	h := ops.NewHealth()
	h.Register("bad", func(context.Context) error { return errors.New("nope") })
	api := ops.NewAPI(ops.WithHealth(h))
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()
	if _, code := get(t, srv.URL+"/readyz", ""); code != http.StatusServiceUnavailable {
		t.Errorf("/readyz unhealthy code = %d want 503", code)
	}
}

func TestAPIAuth(t *testing.T) {
	p := rbac.NewPolicy(rbac.WithIterations(4096)) // low work factor keeps the test fast
	p.AddRole("admin", "admin:read")
	p.AddUser("alice", "admin")
	p.AddUser("bob") // no roles
	if err := p.SetPassword("alice", "secret"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPassword("bob", "secret"); err != nil {
		t.Fatal(err)
	}

	api := ops.NewAPI(ops.WithAuth(p), ops.WithInfo(map[string]string{"version": "1"}))
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	// No credentials -> 401.
	if _, code := get(t, srv.URL+"/info", ""); code != http.StatusUnauthorized {
		t.Errorf("/info no-auth code = %d want 401", code)
	}
	// Valid user with permission -> 200.
	if _, code := get(t, srv.URL+"/info", "alice:secret"); code != http.StatusOK {
		t.Errorf("/info alice code = %d want 200", code)
	}
	// Valid user without permission -> 403.
	if _, code := get(t, srv.URL+"/info", "bob:secret"); code != http.StatusForbidden {
		t.Errorf("/info bob code = %d want 403", code)
	}
	// Wrong password -> 401.
	if _, code := get(t, srv.URL+"/info", "alice:wrong"); code != http.StatusUnauthorized {
		t.Errorf("/info wrong-pw code = %d want 401", code)
	}
	// Public endpoints stay open even with auth configured.
	if _, code := get(t, srv.URL+"/healthz", ""); code != http.StatusOK {
		t.Errorf("/healthz code = %d want 200", code)
	}
}

// get performs a GET, optionally with "user:pass" basic auth, returning body and status.
func get(t *testing.T, url, auth string) (string, int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		u, p, _ := strings.Cut(auth, ":")
		req.SetBasicAuth(u, p)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String(), resp.StatusCode
}
