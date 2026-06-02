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

package gateway_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/gateway"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/mux"
	"github.com/teqpace-services/isopace/packager"
)

var codec = iso8583.NewCodec(packager.ISO87A())

// fakeHost is an upstream Forwarder: it records the last request it received and
// answers 0200 with an approved 0210, echoing DE 11 + DE 41.
type fakeHost struct {
	mu      sync.Mutex
	lastReq *iso8583.Message
	err     error // if set, Request returns it
}

func (h *fakeHost) Request(_ context.Context, req []byte) ([]byte, error) {
	if h.err != nil {
		return nil, h.err
	}
	m, err := codec.Unmarshal(req)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.lastReq = m
	h.mu.Unlock()

	resp := iso8583.New(packager.ISO87A())
	_ = resp.Set(0, "0210")
	if stan, e := iso8583.Get[int64](m, 11); e == nil {
		_ = resp.Set(11, stan)
	}
	if term, e := iso8583.Get[string](m, 41); e == nil {
		_ = resp.Set(41, term)
	}
	_ = resp.Set(39, "00")
	_ = resp.Set(38, "654321")
	return codec.Marshal(resp, nil)
}

func (h *fakeHost) received() *iso8583.Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastReq
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// startGateway starts a gateway on a loopback port and returns it.
func startGateway(t *testing.T, cfg gateway.Config) *gateway.Gateway {
	t.Helper()
	cfg.Addr = "127.0.0.1:0"
	cfg.Codec = codec
	if cfg.Log == nil {
		cfg.Log = quietLog()
	}
	g, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { g.Stop(context.Background()) })
	return g
}

// dialClient connects to the gateway as an inbound peer (terminal/acquirer).
func dialClient(t *testing.T, addr string) *mux.Mux {
	t.Helper()
	cl, err := link.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	m := mux.New(cl, mux.FieldKeyer(codec, 11, 41), mux.WithTimeout(2*time.Second))
	t.Cleanup(func() { m.Close() })
	return m
}

func authReq(t *testing.T, stan int, amountMinor int64) []byte {
	t.Helper()
	m := iso8583.New(packager.ISO87A())
	_ = m.Set(0, "0200")
	_ = m.Set(2, "4012345678909")
	_ = m.Set(3, "000000")
	_ = m.Set(4, amountMinor)
	_ = m.Set(11, int64(stan))
	_ = m.Set(41, "TERM0001")
	_ = m.Set(49, "566")
	w, err := codec.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return w
}

func decode(t *testing.T, frame []byte) *iso8583.Message {
	t.Helper()
	m, err := codec.Unmarshal(frame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

// TestGatewayRoutesAndForwards: a request is forwarded to the routed host and the
// response comes back correlated.
func TestGatewayRoutesAndForwards(t *testing.T) {
	host := &fakeHost{}
	g := startGateway(t, gateway.Config{
		Name:  "pos",
		Route: func(context.Context, *iso8583.Message) (gateway.Forwarder, error) { return host, nil },
	})
	cl := dialClient(t, g.Addr())

	resp, err := cl.Request(context.Background(), authReq(t, 1, 100_00))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	m := decode(t, resp)
	if mti, _ := iso8583.Get[string](m, 0); mti != "0210" {
		t.Errorf("MTI = %q want 0210", mti)
	}
	if rc, _ := iso8583.Get[string](m, 39); rc != "00" {
		t.Errorf("rc = %q want 00", rc)
	}
	if stan, _ := iso8583.Get[int64](m, 11); stan != 1 {
		t.Errorf("STAN = %d want 1 (correlation)", stan)
	}
}

// TestGatewayTransformsRequestAndResponse: BeforeForward edits the outbound
// request (the host sees it) and AfterForward edits the response (the client
// sees it).
func TestGatewayTransformsRequestAndResponse(t *testing.T) {
	host := &fakeHost{}
	g := startGateway(t, gateway.Config{
		Name:  "pos",
		Route: func(context.Context, *iso8583.Message) (gateway.Forwarder, error) { return host, nil },
		BeforeForward: func(_ context.Context, req *iso8583.Message) (*iso8583.Message, error) {
			// Stamp the acquiring institution id before forwarding.
			if err := req.Set(32, int64(99001)); err != nil {
				return nil, err
			}
			return req, nil
		},
		AfterForward: func(_ context.Context, _, resp *iso8583.Message) (*iso8583.Message, error) {
			// Stamp a gateway marker on the response.
			if err := resp.Set(48, "ROUTED-VIA-TEQ"); err != nil {
				return nil, err
			}
			return resp, nil
		},
	})
	cl := dialClient(t, g.Addr())

	resp, err := cl.Request(context.Background(), authReq(t, 2, 50_00))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	// The host saw the injected DE 32.
	got := host.received()
	if got == nil {
		t.Fatal("host received nothing")
	}
	if v, _ := iso8583.Get[int64](got, 32); v != 99001 {
		t.Errorf("host DE 32 = %d want 99001 (request not transformed)", v)
	}

	// The client saw the injected DE 48.
	m := decode(t, resp)
	if v, _ := iso8583.Get[string](m, 48); v != "ROUTED-VIA-TEQ" {
		t.Errorf("client DE 48 = %q want ROUTED-VIA-TEQ (response not transformed)", v)
	}
}

// TestGatewayNoRoute: an unroutable request gets the OnError reply.
func TestGatewayNoRoute(t *testing.T) {
	g := startGateway(t, gateway.Config{
		Name:  "pos",
		Route: func(context.Context, *iso8583.Message) (gateway.Forwarder, error) { return nil, gateway.ErrNoRoute },
		OnError: func(_ context.Context, req *iso8583.Message, _ error) (*iso8583.Message, error) {
			resp := iso8583.New(packager.ISO87A())
			_ = resp.Set(0, "0210")
			if stan, e := iso8583.Get[int64](req, 11); e == nil {
				_ = resp.Set(11, stan)
			}
			if term, e := iso8583.Get[string](req, 41); e == nil {
				_ = resp.Set(41, term)
			}
			_ = resp.Set(39, "91") // issuer or switch inoperative
			return resp, nil
		},
	})
	cl := dialClient(t, g.Addr())

	resp, err := cl.Request(context.Background(), authReq(t, 3, 10_00))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if rc, _ := iso8583.Get[string](decode(t, resp), 39); rc != "91" {
		t.Errorf("rc = %q want 91 (no-route decline)", rc)
	}
}

// TestGatewayForwardError: an upstream failure routes to OnError too.
func TestGatewayForwardError(t *testing.T) {
	host := &fakeHost{err: errors.New("upstream down")}
	g := startGateway(t, gateway.Config{
		Name:  "pos",
		Route: func(context.Context, *iso8583.Message) (gateway.Forwarder, error) { return host, nil },
		OnError: func(_ context.Context, req *iso8583.Message, _ error) (*iso8583.Message, error) {
			resp := iso8583.New(packager.ISO87A())
			_ = resp.Set(0, "0210")
			if stan, e := iso8583.Get[int64](req, 11); e == nil {
				_ = resp.Set(11, stan)
			}
			if term, e := iso8583.Get[string](req, 41); e == nil {
				_ = resp.Set(41, term)
			}
			_ = resp.Set(39, "91")
			return resp, nil
		},
	})
	cl := dialClient(t, g.Addr())

	resp, err := cl.Request(context.Background(), authReq(t, 4, 10_00))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if rc, _ := iso8583.Get[string](decode(t, resp), 39); rc != "91" {
		t.Errorf("rc = %q want 91 (forward-error decline)", rc)
	}
}

func TestGatewayValidation(t *testing.T) {
	route := func(context.Context, *iso8583.Message) (gateway.Forwarder, error) { return nil, nil }
	cases := map[string]gateway.Config{
		"no name":  {Addr: "x:1", Codec: codec, Route: route},
		"no addr":  {Name: "g", Codec: codec, Route: route},
		"no codec": {Name: "g", Addr: "x:1", Route: route},
		"no route": {Name: "g", Addr: "x:1", Codec: codec},
	}
	for name, cfg := range cases {
		if _, err := gateway.New(cfg); err == nil {
			t.Errorf("%s: New err = nil want error", name)
		}
	}
}
