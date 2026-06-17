// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction package.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.

package connector_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/connector"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/listener"
	"github.com/teqpace-services/isopace/mux"
	"github.com/teqpace-services/isopace/packager"
)

var codec = iso8583.NewCodec(packager.ISO87A())

// mockOpts tunes the test switch behaviour.
type mockOpts struct {
	dropAfter int    // close the connection after this many responses (0 = never)
	onSignOn  func() // called when a 0800 sign-on is received
	onEcho    func() // called when a 0800 echo (with DE 70) is received
}

// startMock runs an in-process switch that answers 0200->0210 and 0800->0810,
// echoing DE 11 + DE 41 so responses correlate. It returns the listen address.
func startMock(t *testing.T, opts mockOpts) string {
	t.Helper()
	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	go srv.Serve(func(l *link.Link) {
		served := 0
		for {
			req, err := l.Receive()
			if err != nil {
				return
			}
			m, err := codec.Unmarshal(req)
			if err != nil {
				return
			}
			mti, _ := iso8583.Get[string](m, 0)
			respMTI := "0210"
			if mti == "0800" {
				respMTI = "0810"
				if _, hasNMIC := m.Get(70); hasNMIC {
					if opts.onEcho != nil {
						opts.onEcho()
					}
				} else if opts.onSignOn != nil {
					opts.onSignOn()
				}
			}
			resp := iso8583.New(packager.ISO87A())
			_ = resp.Set(0, respMTI)
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
			if served++; opts.dropAfter > 0 && served >= opts.dropAfter {
				l.Close()
				return
			}
		}
	})
	return srv.Addr().String()
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// authReq builds a 0200 with STAN + terminal so it correlates.
func authReq(stan int) []byte {
	m := iso8583.New(packager.ISO87A())
	_ = m.Set(0, "0200")
	_ = m.Set(11, int64(stan))
	_ = m.Set(41, "TERM0001")
	_ = m.Set(4, int64(1000))
	w, _ := codec.Marshal(m, nil)
	return w
}

// netMgmt builds a 0800 network-management message; withNMIC adds DE 70 so the
// mock can tell an echo from a sign-on.
func netMgmt(stan int, withNMIC bool) []byte {
	m := iso8583.New(packager.ISO87A())
	_ = m.Set(0, "0800")
	_ = m.Set(11, int64(stan))
	_ = m.Set(41, "TERM0001")
	if withNMIC {
		_ = m.Set(70, int64(301)) // echo test
	}
	w, _ := codec.Marshal(m, nil)
	return w
}

func newConnector(t *testing.T, cfg connector.Config) *connector.Connector {
	t.Helper()
	if cfg.Keyer == nil {
		cfg.Keyer = mux.FieldKeyer(codec, 11, 41)
	}
	if cfg.Log == nil {
		cfg.Log = quietLog()
	}
	c, err := connector.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { c.Stop(context.Background()) })
	return c
}

func waitConnected(t *testing.T, c *connector.Connector, want bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if c.Connected() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("connector Connected=%v never reached", want)
}

func responseRC(t *testing.T, frame []byte) string {
	t.Helper()
	m, err := codec.Unmarshal(frame)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	rc, _ := iso8583.Get[string](m, 39)
	return rc
}

func TestConnectorConnectsAndRequests(t *testing.T) {
	addr := startMock(t, mockOpts{})
	c := newConnector(t, connector.Config{Name: "isw", Addr: addr, Timeout: 2 * time.Second})

	waitConnected(t, c, true)
	resp, err := c.Request(context.Background(), authReq(1))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if rc := responseRC(t, resp); rc != "00" {
		t.Errorf("rc = %q want 00", rc)
	}
}

func TestConnectorRequestWhenDown(t *testing.T) {
	// Nothing is listening here, so the connector never comes up.
	c := newConnector(t, connector.Config{
		Name: "dead", Addr: "127.0.0.1:1", MinBackoff: 20 * time.Millisecond, MaxBackoff: 40 * time.Millisecond,
	})
	if _, err := c.Request(context.Background(), authReq(1)); !errors.Is(err, connector.ErrNotConnected) {
		t.Errorf("Request err = %v want ErrNotConnected", err)
	}
}

func TestConnectorReconnectsAfterDrop(t *testing.T) {
	// The mock drops the connection after each single response; the connector must
	// re-establish it for the next request.
	addr := startMock(t, mockOpts{dropAfter: 1})
	c := newConnector(t, connector.Config{
		Name: "up", Addr: addr, Timeout: 2 * time.Second,
		MinBackoff: 20 * time.Millisecond, MaxBackoff: 100 * time.Millisecond,
	})

	waitConnected(t, c, true)
	if _, err := c.Request(context.Background(), authReq(1)); err != nil {
		t.Fatalf("first Request: %v", err)
	}

	// The server closed the link after responding; the connector reconnects.
	waitConnected(t, c, false) // observe the drop
	waitConnected(t, c, true)  // and the recovery
	if _, err := c.Request(context.Background(), authReq(2)); err != nil {
		t.Fatalf("Request after reconnect: %v", err)
	}
}

func TestConnectorOnConnectSignOn(t *testing.T) {
	var signOns atomic.Int32
	addr := startMock(t, mockOpts{onSignOn: func() { signOns.Add(1) }})

	c := newConnector(t, connector.Config{
		Name: "isw", Addr: addr, Timeout: 2 * time.Second,
		OnConnect: func(ctx context.Context, m *mux.Mux) error {
			_, err := m.Request(ctx, netMgmt(9001, false)) // sign-on (no DE 70)
			return err
		},
	})

	waitConnected(t, c, true)
	if got := signOns.Load(); got != 1 {
		t.Errorf("sign-ons = %d want 1", got)
	}
	// Normal traffic still flows after sign-on.
	if _, err := c.Request(context.Background(), authReq(1)); err != nil {
		t.Fatalf("Request after sign-on: %v", err)
	}
}

func TestConnectorOnConnectFailureRetries(t *testing.T) {
	addr := startMock(t, mockOpts{})
	var attempts atomic.Int32
	c := newConnector(t, connector.Config{
		Name: "isw", Addr: addr, Timeout: time.Second,
		MinBackoff: 20 * time.Millisecond, MaxBackoff: 40 * time.Millisecond,
		OnConnect: func(context.Context, *mux.Mux) error {
			// Fail the first two sign-on attempts, succeed after.
			if attempts.Add(1) <= 2 {
				return errors.New("sign-on declined")
			}
			return nil
		},
	})

	waitConnected(t, c, true)
	if got := attempts.Load(); got < 3 {
		t.Errorf("sign-on attempts = %d want >= 3 (retried)", got)
	}
}

func TestConnectorKeepalive(t *testing.T) {
	var echoes atomic.Int32
	addr := startMock(t, mockOpts{onEcho: func() { echoes.Add(1) }})

	var stan atomic.Int32
	stan.Store(7000)
	c := newConnector(t, connector.Config{
		Name: "isw", Addr: addr, Timeout: time.Second,
		KeepaliveInterval: 30 * time.Millisecond,
		Keepalive: func(ctx context.Context, m *mux.Mux) error {
			_, err := m.Request(ctx, netMgmt(int(stan.Add(1)), true)) // echo (DE 70)
			return err
		},
	})

	waitConnected(t, c, true)
	for i := 0; i < 200; i++ {
		if echoes.Load() >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("keepalive echoes = %d want >= 2", echoes.Load())
}

func TestConnectorValidation(t *testing.T) {
	cases := map[string]connector.Config{
		"no name":  {Addr: "x:1", Keyer: mux.FieldKeyer(codec, 11)},
		"no addr":  {Name: "x", Keyer: mux.FieldKeyer(codec, 11)},
		"no keyer": {Name: "x", Addr: "x:1"},
	}
	for name, cfg := range cases {
		if _, err := connector.New(cfg); err == nil {
			t.Errorf("%s: New err = nil want error", name)
		}
	}
}
