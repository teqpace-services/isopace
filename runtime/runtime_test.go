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

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	rt "github.com/teqpace-services/isopace/runtime"
)

// eventLog records lifecycle events from test components in order.
type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (e *eventLog) add(s string) {
	e.mu.Lock()
	e.events = append(e.events, s)
	e.mu.Unlock()
}

func (e *eventLog) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.events...)
}

// recorder is a Component that logs start/stop and can fail its Start.
type recorder struct {
	name     string
	log      *eventLog
	startErr error
	stopErr  error
	v        int
}

func (r *recorder) Name() string { return r.name }

func (r *recorder) Start(context.Context) error {
	if r.startErr != nil {
		return r.startErr
	}
	r.log.add(r.name + ":start")
	return nil
}

func (r *recorder) Stop(context.Context) error {
	r.log.add(r.name + ":stop")
	return r.stopErr
}

func testHost(t *testing.T, log *eventLog) *rt.Host {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	return rt.New(rt.WithLogger(quiet))
}

func TestHostStartStopOrder(t *testing.T) {
	log := &eventLog{}
	h := testHost(t, log)
	for _, n := range []string{"a", "b", "c"} {
		if err := h.Register(&recorder{name: n, log: log}); err != nil {
			t.Fatalf("Register %s: %v", n, err)
		}
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	got := log.snapshot()
	want := []string{"a:start", "b:start", "c:start", "c:stop", "b:stop", "a:stop"}
	if !slices.Equal(got, want) {
		t.Errorf("lifecycle order = %v want %v", got, want)
	}
}

func TestHostStartUnwindsOnFailure(t *testing.T) {
	log := &eventLog{}
	h := testHost(t, log)
	boom := errors.New("boom")
	_ = h.Register(&recorder{name: "a", log: log})
	_ = h.Register(&recorder{name: "b", log: log, startErr: boom})
	_ = h.Register(&recorder{name: "c", log: log})

	err := h.Start(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("Start err = %v want boom", err)
	}
	// a started then must be stopped during unwind; b never fully started; c never reached.
	got := log.snapshot()
	want := []string{"a:start", "a:stop"}
	if !slices.Equal(got, want) {
		t.Errorf("unwind events = %v want %v", got, want)
	}
}

func TestHostStartUnwindReportsStopErrors(t *testing.T) {
	log := &eventLog{}
	h := testHost(t, log)
	startBoom := errors.New("start boom")
	stopFail := errors.New("stop fail")
	_ = h.Register(&recorder{name: "a", log: log, stopErr: stopFail})
	_ = h.Register(&recorder{name: "b", log: log, startErr: startBoom})

	err := h.Start(context.Background())
	if !errors.Is(err, startBoom) {
		t.Errorf("Start err missing start cause: %v", err)
	}
	if !errors.Is(err, stopFail) {
		t.Errorf("Start err missing unwind stop error: %v", err)
	}
}

func TestDeployerKeepsOldOnFailedRebuild(t *testing.T) {
	log := &eventLog{}
	h := testHost(t, log)
	_ = h.Start(context.Background())
	defer h.Stop(context.Background())

	var builds atomic.Int32
	reg := rt.NewRegistry()
	reg.Register("rec", func(name string, _ json.RawMessage, _ *rt.Env) (rt.Component, error) {
		if builds.Add(1) > 1 {
			return nil, errors.New("bad config") // every rebuild fails
		}
		return &recorder{name: name, log: log}, nil
	})

	dir := t.TempDir()
	desc := filepath.Join(dir, "a.json")
	if err := os.WriteFile(desc, []byte(`{"name":"svc","type":"rec","enabled":true,"config":{"v":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dp := rt.NewDeployer(h, reg, dir)
	if err := dp.Scan(context.Background()); err != nil {
		t.Fatalf("Scan(1): %v", err)
	}
	if !slices.Contains(h.Components(), "svc") {
		t.Fatal("svc not deployed")
	}

	// Change the descriptor; the rebuild will fail. The running component must
	// stay up rather than be torn down for a replacement that never builds.
	if err := os.WriteFile(desc, []byte(`{"name":"svc","type":"rec","enabled":true,"config":{"v":2}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dp.Scan(context.Background()); err == nil {
		t.Error("Scan should report the rebuild failure")
	}
	if !slices.Contains(h.Components(), "svc") {
		t.Error("old component torn down despite failed rebuild")
	}
	if slices.Contains(log.snapshot(), "svc:stop") {
		t.Errorf("old component stopped on failed rebuild: %v", log.snapshot())
	}
}

func TestConfigEnvOverrideSkipsScalarCollision(t *testing.T) {
	// "server" is a scalar; an override implying server.port must not clobber it.
	c, err := rt.Parse([]byte(`{"server":"plain"}`),
		rt.WithEnvPrefix("ISO"), rt.WithEnviron([]string{"ISO_SERVER_PORT=8080"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.String("server", ""); got != "plain" {
		t.Errorf("server = %q want plain (scalar preserved)", got)
	}
	if _, ok := c.Get("server.port"); ok {
		t.Errorf("override clobbered scalar into a map")
	}
}

func TestConfigDurationOverflow(t *testing.T) {
	c, err := rt.Parse([]byte(`{"t": 1e19}`)) // *1e9 overflows int64
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Duration("t", 7*time.Second); got != 7*time.Second {
		t.Errorf("Duration overflow = %v want default 7s", got)
	}
	c2, _ := rt.Parse([]byte(`{"t": 5}`)) // sane: seconds
	if got := c2.Duration("t", 0); got != 5*time.Second {
		t.Errorf("Duration(5) = %v want 5s", got)
	}
}

func TestHostDuplicateRegister(t *testing.T) {
	h := testHost(t, &eventLog{})
	_ = h.Register(&recorder{name: "dup", log: &eventLog{}})
	if err := h.Register(&recorder{name: "dup", log: &eventLog{}}); !errors.Is(err, rt.ErrDuplicate) {
		t.Errorf("duplicate Register = %v want ErrDuplicate", err)
	}
}

func TestHostDeployUndeployWhileRunning(t *testing.T) {
	log := &eventLog{}
	h := testHost(t, log)
	if err := h.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer h.Stop(context.Background())

	if err := h.Deploy(context.Background(), &recorder{name: "live", log: log}); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if got := h.Components(); !slices.Contains(got, "live") {
		t.Errorf("Components = %v missing live", got)
	}
	if err := h.Undeploy(context.Background(), "live"); err != nil {
		t.Fatalf("Undeploy: %v", err)
	}
	if err := h.Undeploy(context.Background(), "ghost"); !errors.Is(err, rt.ErrNotFound) {
		t.Errorf("Undeploy ghost = %v want ErrNotFound", err)
	}
	want := []string{"live:start", "live:stop"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Errorf("deploy events = %v want %v", got, want)
	}
}

func TestHostRunStopsOnContextCancel(t *testing.T) {
	log := &eventLog{}
	h := testHost(t, log)
	_ = h.Register(&recorder{name: "svc", log: log})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.Run(ctx) }()
	// Let it start, then cancel and expect a clean stop.
	if !waitFor(200*time.Millisecond, func() bool { return slices.Contains(log.snapshot(), "svc:start") }) {
		t.Fatal("service did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if got := log.snapshot(); !slices.Contains(got, "svc:stop") {
		t.Errorf("service not stopped on cancel: %v", got)
	}
}

func TestConfigGettersAndEnvOverride(t *testing.T) {
	js := `{
		"link":   {"timeout": "5s", "pool": 2},
		"logging":{"level": "info"},
		"flag":   true
	}`
	env := []string{
		"ISO_LINK_POOL=4",
		"ISO_NEW_KEY=hello",
		"ISO_FLAG=false",
		"OTHER_IGNORED=1",
	}
	c, err := rt.Parse([]byte(js), rt.WithEnvPrefix("ISO"), rt.WithEnviron(env))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := c.Int("link.pool", 0); got != 4 {
		t.Errorf("link.pool = %d want 4 (env override)", got)
	}
	if got := c.Duration("link.timeout", 0); got != 5*time.Second {
		t.Errorf("link.timeout = %v want 5s", got)
	}
	if got := c.String("logging.level", ""); got != "info" {
		t.Errorf("logging.level = %q want info", got)
	}
	if c.Bool("flag", true) {
		t.Errorf("flag = true want false (env override)")
	}
	if got := c.String("new.key", ""); got != "hello" {
		t.Errorf("new.key = %q want hello", got)
	}
	if _, ok := c.Get("nope"); ok {
		t.Errorf("missing key reported present")
	}
}

func TestConfigUnmarshal(t *testing.T) {
	c, err := rt.Parse([]byte(`{"server":{"addr":"127.0.0.1:5000","pool":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	var srv struct {
		Addr string `json:"addr"`
		Pool int    `json:"pool"`
	}
	if err := c.UnmarshalKey("server", &srv); err != nil {
		t.Fatalf("UnmarshalKey: %v", err)
	}
	if srv.Addr != "127.0.0.1:5000" || srv.Pool != 3 {
		t.Errorf("unmarshalled %+v", srv)
	}
}

func TestConfigReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"level":"info"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := rt.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.String("level", ""); got != "info" {
		t.Fatalf("initial level = %q", got)
	}
	if err := os.WriteFile(path, []byte(`{"level":"debug"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := c.String("level", ""); got != "debug" {
		t.Errorf("after reload level = %q want debug", got)
	}
}

func TestConfigWatcher(t *testing.T) {
	baseline := goroutineBaseline()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"n":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := rt.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := make(chan struct{}, 4)
	w := rt.NewConfigWatcher(c,
		rt.WithWatchInterval(10*time.Millisecond),
		rt.WithWatchLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		rt.OnReload(func(*rt.Config) { reloaded <- struct{}{} }))
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Change content (different size) so the stamp differs regardless of mtime resolution.
	if err := os.WriteFile(path, []byte(`{"n":222222}`), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reloaded:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not fire reload")
	}
	if got := c.Int("n", 0); got != 222222 {
		t.Errorf("reloaded n = %d want 222222", got)
	}
	if err := w.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	assertNoLeak(t, baseline)
}

func TestDeployerReconcile(t *testing.T) {
	log := &eventLog{}
	h := testHost(t, log)
	if err := h.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer h.Stop(context.Background())

	reg := rt.NewRegistry()
	reg.Register("rec", func(name string, cfg json.RawMessage, _ *rt.Env) (rt.Component, error) {
		var c struct {
			V int `json:"v"`
		}
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		return &recorder{name: name, log: log, v: c.V}, nil
	})

	dir := t.TempDir()
	desc := filepath.Join(dir, "alpha.json")
	write := func(body string) {
		if err := os.WriteFile(desc, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"name":"alpha","type":"rec","enabled":true,"config":{"v":1}}`)

	dp := rt.NewDeployer(h, reg, dir)
	if err := dp.Scan(context.Background()); err != nil {
		t.Fatalf("Scan(1): %v", err)
	}
	if got := h.Components(); !slices.Contains(got, "alpha") {
		t.Fatalf("after first scan components = %v", got)
	}

	// Re-scan with no change: must be idempotent (no extra start/stop).
	if err := dp.Scan(context.Background()); err != nil {
		t.Fatalf("Scan(noop): %v", err)
	}

	// Change the descriptor: must redeploy (old stop, new start).
	write(`{"name":"alpha","type":"rec","enabled":true,"config":{"v":2}}`)
	if err := dp.Scan(context.Background()); err != nil {
		t.Fatalf("Scan(2): %v", err)
	}

	// Remove the descriptor: must undeploy.
	if err := os.Remove(desc); err != nil {
		t.Fatal(err)
	}
	if err := dp.Scan(context.Background()); err != nil {
		t.Fatalf("Scan(rm): %v", err)
	}
	if got := h.Components(); slices.Contains(got, "alpha") {
		t.Errorf("alpha not undeployed: %v", got)
	}

	want := []string{"alpha:start", "alpha:stop", "alpha:start", "alpha:stop"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Errorf("deployer lifecycle = %v want %v", got, want)
	}
}

func TestDeployerDisabledAndUnknownType(t *testing.T) {
	log := &eventLog{}
	h := testHost(t, log)
	_ = h.Start(context.Background())
	defer h.Stop(context.Background())

	reg := rt.NewRegistry()
	reg.Register("rec", func(name string, _ json.RawMessage, _ *rt.Env) (rt.Component, error) {
		return &recorder{name: name, log: log}, nil
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "off.json"),
		[]byte(`{"name":"off","type":"rec","enabled":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"),
		[]byte(`{"name":"bad","type":"nope","enabled":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dp := rt.NewDeployer(h, reg, dir)
	err := dp.Scan(context.Background())
	if err == nil {
		t.Errorf("Scan should report the unknown-type error")
	}
	if got := h.Components(); slices.Contains(got, "off") {
		t.Errorf("disabled descriptor was deployed: %v", got)
	}
	if got := log.snapshot(); len(got) != 0 {
		t.Errorf("no components should have started, got %v", got)
	}
}

func TestObservers(t *testing.T) {
	// NopObserver must be inert and never panic.
	ctx, span := rt.NopObserver{}.StartSpan(context.Background(), "x", rt.A("k", "v"))
	span.SetAttr(rt.A("a", 1))
	span.SetError(errors.New("e"))
	span.End()
	rt.NopObserver{}.Counter("c").Add(1)
	rt.NopObserver{}.Histogram("h").Observe(1.5)
	_ = ctx

	// SlogObserver must emit without panicking.
	obs := rt.SlogObserver{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	_, s := obs.StartSpan(context.Background(), "tx", rt.A("mti", "0200"))
	s.SetError(errors.New("decline"))
	s.End()
	obs.Counter("tx.count").Add(2, rt.A("rc", "00"))
	obs.Histogram("tx.latency").Observe(12.5)

	// Env substitutes safe defaults for nil fields.
	var e *rt.Env
	if e.Logger() == nil {
		t.Error("nil Env.Logger returned nil")
	}
	if e.Observer() == nil {
		t.Error("nil Env.Observer returned nil")
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug, "INFO": slog.LevelInfo,
		"warn": slog.LevelWarn, "error": slog.LevelError,
	}
	for in, want := range cases {
		if got := rt.ParseLevel(in, slog.LevelInfo); got != want {
			t.Errorf("ParseLevel(%q) = %v want %v", in, got, want)
		}
	}
	if got := rt.ParseLevel("nonsense", slog.LevelWarn); got != slog.LevelWarn {
		t.Errorf("ParseLevel fallback = %v want Warn", got)
	}
}

func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

func goroutineBaseline() int {
	time.Sleep(20 * time.Millisecond)
	runtime.GC()
	return runtime.NumGoroutine()
}

func assertNoLeak(t *testing.T, baseline int) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if runtime.NumGoroutine() <= baseline {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("goroutine leak: %d running, baseline %d", runtime.NumGoroutine(), baseline)
}
