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

package link_test

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/link"
)

// acceptAll runs a bare accept loop, holding connections until shutdown.
func acceptAll(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	return ln.Addr().String(), func() {
		close(done)
		ln.Close()
		mu.Lock()
		for _, c := range conns {
			c.Close()
		}
		mu.Unlock()
		wg.Wait()
	}
}

func TestPoolRoundRobinAndReconnect(t *testing.T) {
	addr, stop := acceptAll(t)
	defer stop()

	dial := func() (*link.Link, error) { return link.Dial("tcp", addr) }
	p := link.NewPool(2, dial, link.WithBackoff(5*time.Millisecond, 20*time.Millisecond))
	defer p.Close()

	a, err := p.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	b, err := p.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if a == b {
		t.Errorf("round-robin returned the same link twice")
	}

	// Discard one and confirm the slot reconnects.
	p.Discard(a)
	if !eventually(t, 2*time.Second, func() bool {
		l, err := p.Get()
		return err == nil && l != nil
	}) {
		t.Errorf("pool did not recover a healthy link after Discard")
	}
}

func TestPoolNoHealthy(t *testing.T) {
	// Dial a closed port: every slot stays in reconnect, so Get reports no health.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close() // nothing listening now

	dial := func() (*link.Link, error) { return link.Dial("tcp", addr, link.WithDialTimeout(50*time.Millisecond)) }
	p := link.NewPool(2, dial, link.WithBackoff(time.Second, time.Second))
	defer p.Close()

	if _, err := p.Get(); !errors.Is(err, link.ErrNoHealthy) {
		t.Errorf("Get on dead target = %v want ErrNoHealthy", err)
	}
}

func TestPoolClosed(t *testing.T) {
	addr, stop := acceptAll(t)
	defer stop()
	p := link.NewPool(1, func() (*link.Link, error) { return link.Dial("tcp", addr) })
	p.Close()
	if _, err := p.Get(); !errors.Is(err, link.ErrPoolClosed) {
		t.Errorf("Get after Close = %v want ErrPoolClosed", err)
	}
}

func eventually(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}
