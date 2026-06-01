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

package listener_test

import (
	"bytes"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/listener"
)

// echo reads frames and writes them straight back until the link closes.
func echo(l *link.Link) {
	for {
		msg, err := l.Receive()
		if err != nil {
			return
		}
		if err := l.Send(msg); err != nil {
			return
		}
	}
}

func TestListenerServeAndClose(t *testing.T) {
	baseline := goroutineBaseline()

	s, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var serveWG sync.WaitGroup
	serveWG.Add(1)
	go func() {
		defer serveWG.Done()
		if err := s.Serve(echo); err != nil {
			t.Errorf("Serve: %v", err)
		}
	}()

	// A few concurrent clients each round-trip a frame.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := link.Dial("tcp", s.Addr().String())
			if err != nil {
				t.Errorf("Dial: %v", err)
				return
			}
			defer c.Close()
			want := []byte("0800netmgmt")
			if err := c.Send(want); err != nil {
				t.Errorf("Send: %v", err)
				return
			}
			got, err := c.Receive()
			if err != nil {
				t.Errorf("Receive: %v", err)
				return
			}
			if !bytes.Equal(got, want) {
				t.Errorf("echo = % X want % X", got, want)
			}
		}()
	}
	wg.Wait()

	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	serveWG.Wait() // Serve must return after Close

	assertNoLeak(t, baseline)
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
