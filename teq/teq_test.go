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

package teq_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/connector"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/listener"
	"github.com/teqpace-services/isopace/mux"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/runtime"
	"github.com/teqpace-services/isopace/teq"
)

var codec = iso8583.NewCodec(packager.ISO87A())

// startEcho runs an in-process switch that answers 0200 with an approved 0210,
// echoing DE 11 + DE 41 for correlation. Returns its address.
func startEcho(t *testing.T) string {
	t.Helper()
	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	go srv.Serve(func(l *link.Link) {
		for {
			req, err := l.Receive()
			if err != nil {
				return
			}
			m, err := codec.Unmarshal(req)
			if err != nil {
				return
			}
			resp := iso8583.New(packager.ISO87A())
			_ = resp.Set(0, "0210")
			if stan, e := iso8583.Get[int64](m, 11); e == nil {
				_ = resp.Set(11, stan)
			}
			if term, e := iso8583.Get[string](m, 41); e == nil {
				_ = resp.Set(41, term)
			}
			_ = resp.Set(39, "00")
			w, err := codec.Marshal(resp, nil)
			if err != nil {
				return
			}
			if err := l.Send(w); err != nil {
				return
			}
		}
	})
	return srv.Addr().String()
}

func authReq(stan int) []byte {
	m := iso8583.New(packager.ISO87A())
	_ = m.Set(0, "0200")
	_ = m.Set(11, int64(stan))
	_ = m.Set(41, "TERM0001")
	w, _ := codec.Marshal(m, nil)
	return w
}

func quietQ(t *testing.T, opts ...teq.Option) *teq.Q {
	t.Helper()
	opts = append(opts, teq.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	return teq.New(opts...)
}

func waitConnected(t *testing.T, c *connector.Connector) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if c != nil && c.Connected() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("switch never connected")
}

// TestQSwitchBeforeStart shows the headline flow: register switches from code,
// start the container, send through a named switch.
func TestQSwitchBeforeStart(t *testing.T) {
	addr := startEcho(t)
	q := quietQ(t)

	isw, err := q.Switch(connector.Config{
		Name: "isw", Addr: addr, Keyer: mux.FieldKeyer(codec, 11, 41), Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if err := q.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer q.Stop(context.Background())

	waitConnected(t, isw)
	if _, err := q.To("isw").Request(context.Background(), authReq(1)); err != nil {
		t.Fatalf("Request: %v", err)
	}
}

// TestQSwitchAfterStart: a switch added to a running container starts at once.
func TestQSwitchAfterStart(t *testing.T) {
	addr := startEcho(t)
	q := quietQ(t)
	if err := q.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer q.Stop(context.Background())

	up, err := q.Switch(connector.Config{
		Name: "up", Addr: addr, Keyer: mux.FieldKeyer(codec, 11, 41), Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	waitConnected(t, up)
	if _, err := up.Request(context.Background(), authReq(2)); err != nil {
		t.Fatalf("Request: %v", err)
	}
}

func TestQDuplicateSwitch(t *testing.T) {
	q := quietQ(t)
	cfg := connector.Config{Name: "isw", Addr: "127.0.0.1:1", Keyer: mux.FieldKeyer(codec, 11, 41)}
	if _, err := q.Switch(cfg); err != nil {
		t.Fatalf("first Switch: %v", err)
	}
	defer q.Stop(context.Background())
	if _, err := q.Switch(cfg); err == nil {
		t.Error("duplicate Switch err = nil want error")
	}
}

func TestQUnknownSwitch(t *testing.T) {
	q := quietQ(t)
	if c := q.To("nope"); c != nil {
		t.Errorf("To(nope) = %v want nil", c)
	}
}

// TestQDeployDir: a switch declared as a JSON descriptor connects and is reachable
// via To.
func TestQDeployDir(t *testing.T) {
	addr := startEcho(t)
	dir := t.TempDir()

	cfg, _ := json.Marshal(map[string]any{"addr": addr, "timeout_ms": 2000})
	desc, _ := json.Marshal(runtime.Descriptor{Name: "isw", Type: "connector", Enabled: true, Config: cfg})
	if err := os.WriteFile(filepath.Join(dir, "isw.json"), desc, 0o600); err != nil {
		t.Fatal(err)
	}

	q := quietQ(t, teq.WithDeployDir(dir), teq.WithScanInterval(50*time.Millisecond))
	if err := q.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer q.Stop(context.Background())

	// Wait for the deployer to materialise and connect the switch.
	for i := 0; i < 300; i++ {
		if c := q.To("isw"); c != nil && c.Connected() {
			if _, err := c.Request(context.Background(), authReq(3)); err != nil {
				t.Fatalf("Request: %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("declared switch never connected")
}
