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

package mux_test

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/listener"
	"github.com/teqpace-services/isopace/mux"
	"github.com/teqpace-services/isopace/packager"
)

// issuer echoes each request back as a 0210 response (flipping the MTI), which
// preserves DE 11 + DE 41 — the correlation key.
func issuer(l *link.Link) {
	for {
		msg, err := l.Receive()
		if err != nil {
			return
		}
		resp := append([]byte(nil), msg...)
		if len(resp) >= 4 {
			resp[2] = '1' // "0200" -> "0210"
		}
		if err := l.Send(resp); err != nil {
			return
		}
	}
}

func authReq(t *testing.T, s *iso8583.Schema, c *iso8583.Codec, stan int) []byte {
	t.Helper()
	m := iso8583.New(s)
	mustSet(t, m, 0, "0200")
	mustSet(t, m, 11, int64(stan))
	mustSet(t, m, 41, "TERM0001")
	w, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return w
}

func mustSet(t *testing.T, m *iso8583.Message, de int, v any) {
	t.Helper()
	if err := m.Set(de, v); err != nil {
		t.Fatalf("Set(%d): %v", de, err)
	}
}

func TestMuxRequestResponse(t *testing.T) {
	baseline := goroutineBaseline()
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)

	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(issuer)

	cl, err := link.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m := mux.New(cl, mux.FieldKeyer(c, 11, 41), mux.WithTimeout(2*time.Second))

	// Concurrent requests with distinct STANs must each get their own response.
	var wg sync.WaitGroup
	for i := 1; i <= 25; i++ {
		wg.Add(1)
		go func(stan int) {
			defer wg.Done()
			resp, err := m.Request(context.Background(), authReq(t, s, c, stan))
			if err != nil {
				t.Errorf("stan %d: %v", stan, err)
				return
			}
			rm, err := c.Unmarshal(resp)
			if err != nil {
				t.Errorf("stan %d decode: %v", stan, err)
				return
			}
			if mti, _ := iso8583.Get[string](rm, 0); mti != "0210" {
				t.Errorf("stan %d response MTI = %q want 0210", stan, mti)
			}
			if got, _ := iso8583.Get[int64](rm, 11); got != int64(stan) {
				t.Errorf("response correlated to wrong STAN: got %d want %d", got, stan)
			}
		}(i)
	}
	wg.Wait()

	m.Close()
	srv.Close()
	assertNoLeak(t, baseline)
}

func TestMuxTimeout(t *testing.T) {
	// Server that reads but never responds.
	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(func(l *link.Link) {
		for {
			if _, err := l.Receive(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	cl, err := link.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m := mux.New(cl, mux.FieldKeyer(c, 11, 41), mux.WithTimeout(100*time.Millisecond))
	defer m.Close()

	_, err = m.Request(context.Background(), authReq(t, s, c, 7))
	if !errors.Is(err, mux.ErrTimeout) {
		t.Errorf("Request err = %v want ErrTimeout", err)
	}
}

func TestMuxCloseUnblocksPending(t *testing.T) {
	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(func(l *link.Link) {
		for {
			if _, err := l.Receive(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	cl, err := link.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m := mux.New(cl, mux.FieldKeyer(c, 11, 41))

	errc := make(chan error, 1)
	go func() {
		_, e := m.Request(context.Background(), authReq(t, s, c, 9))
		errc <- e
	}()
	time.Sleep(50 * time.Millisecond) // let the request register
	m.Close()

	select {
	case e := <-errc:
		if e == nil {
			t.Errorf("pending request should fail when mux closes")
		}
	case <-time.After(2 * time.Second):
		t.Errorf("Close did not unblock the pending request")
	}
}

func TestMuxDoneOnClose(t *testing.T) {
	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	go srv.Serve(func(l *link.Link) {
		for {
			if _, err := l.Receive(); err != nil {
				return
			}
		}
	})

	c := iso8583.NewCodec(packager.ISO87A())
	cl, err := link.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m := mux.New(cl, mux.FieldKeyer(c, 11, 41))

	if m.Err() != nil {
		t.Errorf("Err() = %v want nil while live", m.Err())
	}
	m.Close()
	select {
	case <-m.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed after Close")
	}
	if !errors.Is(m.Err(), mux.ErrClosed) {
		t.Errorf("Err() = %v want ErrClosed", m.Err())
	}
}

func TestMuxDoneOnLinkDeath(t *testing.T) {
	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Server drops the connection right after accepting it.
	go srv.Serve(func(l *link.Link) { l.Close() })

	c := iso8583.NewCodec(packager.ISO87A())
	cl, err := link.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m := mux.New(cl, mux.FieldKeyer(c, 11, 41))
	defer m.Close()
	defer srv.Close()

	select {
	case <-m.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed after link death")
	}
	if m.Err() == nil {
		t.Error("Err() = nil after link death, want the read error")
	}
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
