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

package space_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/space"
)

func TestLocalFIFOAndProbes(t *testing.T) {
	s := space.NewLocal()
	defer s.Close()
	s.Out("q", 1)
	s.Out("q", 2)
	s.Out("q", 3)
	if got := s.Len("q"); got != 3 {
		t.Fatalf("Len = %d want 3", got)
	}
	// Rdp peeks the head without removing.
	if v, ok := s.Rdp("q"); !ok || v != 1 {
		t.Errorf("Rdp = %v,%v want 1,true", v, ok)
	}
	if s.Len("q") != 3 {
		t.Errorf("Rdp removed an entry")
	}
	// Inp removes in FIFO order.
	for _, want := range []int{1, 2, 3} {
		if v, ok := s.Inp("q"); !ok || v != want {
			t.Errorf("Inp = %v,%v want %d,true", v, ok, want)
		}
	}
	if _, ok := s.Inp("q"); ok {
		t.Errorf("Inp on empty key returned ok")
	}
	if s.Len("q") != 0 {
		t.Errorf("queue not drained")
	}
}

func TestLocalBlockingInThenOut(t *testing.T) {
	baseline := goroutineBaseline()
	s := space.NewLocal()

	got := make(chan any, 1)
	go func() {
		v, err := s.In(context.Background(), "k")
		if err != nil {
			t.Errorf("In: %v", err)
			got <- nil
			return
		}
		got <- v
	}()
	// Consumer is blocked; produce and expect it to wake.
	time.Sleep(20 * time.Millisecond)
	s.Out("k", "hello")
	select {
	case v := <-got:
		if v != "hello" {
			t.Errorf("In = %v want hello", v)
		}
	case <-time.After(time.Second):
		t.Fatal("blocking In did not wake on Out")
	}
	s.Close()
	assertNoLeak(t, baseline)
}

func TestLocalRdDoesNotRemove(t *testing.T) {
	s := space.NewLocal()
	defer s.Close()
	s.Out("k", "v")
	if v, err := s.Rd(context.Background(), "k"); err != nil || v != "v" {
		t.Fatalf("Rd = %v,%v", v, err)
	}
	if s.Len("k") != 1 {
		t.Errorf("Rd removed the entry")
	}
	if v, err := s.In(context.Background(), "k"); err != nil || v != "v" {
		t.Errorf("In after Rd = %v,%v", v, err)
	}
}

func TestLocalInContextTimeout(t *testing.T) {
	s := space.NewLocal()
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := s.In(ctx, "absent")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("In timeout err = %v want DeadlineExceeded", err)
	}
}

func TestLocalCloseUnblocksWaiters(t *testing.T) {
	baseline := goroutineBaseline()
	s := space.NewLocal()
	errc := make(chan error, 1)
	go func() {
		_, err := s.In(context.Background(), "k")
		errc <- err
	}()
	time.Sleep(20 * time.Millisecond)
	s.Close()
	select {
	case err := <-errc:
		if !errors.Is(err, space.ErrClosed) {
			t.Errorf("In on close = %v want ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock waiter")
	}
	assertNoLeak(t, baseline)
}

func TestLocalConcurrentProducersConsumers(t *testing.T) {
	baseline := goroutineBaseline()
	s := space.NewLocal()
	const n = 200

	var produced sync.WaitGroup
	for i := 0; i < n; i++ {
		produced.Add(1)
		go func(v int) {
			defer produced.Done()
			s.Out("work", v)
		}(i)
	}

	var mu sync.Mutex
	seen := map[int]int{}
	var consumed sync.WaitGroup
	const consumers = 8
	for c := 0; c < consumers; c++ {
		consumed.Add(1)
		go func() {
			defer consumed.Done()
			for {
				v, err := s.In(context.Background(), "work")
				if err != nil {
					return // ErrClosed once drained
				}
				mu.Lock()
				seen[v.(int)]++
				done := len(seen) == n
				mu.Unlock()
				if done {
					return
				}
			}
		}()
	}

	produced.Wait()
	// Wait until every value has been taken exactly once.
	if !waitFor(2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) == n
	}) {
		t.Fatalf("only %d/%d items consumed", len(seen), n)
	}
	s.Close() // unblock any consumers still parked on In
	consumed.Wait()

	mu.Lock()
	for v, count := range seen {
		if count != 1 {
			t.Errorf("value %d consumed %d times", v, count)
		}
	}
	mu.Unlock()
	assertNoLeak(t, baseline)
}

func TestStorePushPullFIFO(t *testing.T) {
	s := mustOpen(t, filepath.Join(t.TempDir(), "q.log"))
	defer s.Close()
	for _, v := range []string{"a", "b", "c"} {
		if err := s.Push("out", []byte(v)); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}
	if s.Len("out") != 3 {
		t.Fatalf("Len = %d want 3", s.Len("out"))
	}
	for _, want := range []string{"a", "b", "c"} {
		v, ok, err := s.Pullp("out")
		if err != nil || !ok || string(v) != want {
			t.Errorf("Pullp = %q,%v,%v want %q", v, ok, err, want)
		}
	}
	if _, ok, _ := s.Pullp("out"); ok {
		t.Errorf("Pullp on empty returned ok")
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.log")

	s := mustOpen(t, path)
	for _, v := range []string{"m1", "m2", "m3"} {
		if err := s.Push("fwd", []byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	// Take one, leaving m2,m3 queued.
	if v, _, err := s.Pullp("fwd"); err != nil || string(v) != "m1" {
		t.Fatalf("Pullp = %q,%v", v, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen: replay must restore exactly the un-taken entries, in order.
	s2 := mustOpen(t, path)
	defer s2.Close()
	if got := s2.Len("fwd"); got != 2 {
		t.Fatalf("after reopen Len = %d want 2", got)
	}
	for _, want := range []string{"m2", "m3"} {
		v, ok, err := s2.Pullp("fwd")
		if err != nil || !ok || string(v) != want {
			t.Errorf("after reopen Pullp = %q,%v,%v want %q", v, ok, err, want)
		}
	}
}

func TestStoreBlockingPull(t *testing.T) {
	baseline := goroutineBaseline()
	s := mustOpen(t, filepath.Join(t.TempDir(), "q.log"))

	got := make(chan string, 1)
	go func() {
		v, err := s.Pull(context.Background(), "k")
		if err != nil {
			t.Errorf("Pull: %v", err)
			got <- ""
			return
		}
		got <- string(v)
	}()
	time.Sleep(20 * time.Millisecond)
	if err := s.Push("k", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-got:
		if v != "payload" {
			t.Errorf("Pull = %q want payload", v)
		}
	case <-time.After(time.Second):
		t.Fatal("blocking Pull did not wake")
	}
	s.Close()
	assertNoLeak(t, baseline)
}

func TestStoreCompact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.log")
	s := mustOpen(t, path)
	for i := 0; i < 5; i++ {
		if err := s.Push("k", []byte{byte('0' + i)}); err != nil {
			t.Fatal(err)
		}
	}
	// Take three; two remain (3,4).
	for i := 0; i < 3; i++ {
		if _, _, err := s.Pullp("k"); err != nil {
			t.Fatal(err)
		}
	}
	before := fileSize(t, path)
	if err := s.Compact(); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if after := fileSize(t, path); after >= before {
		t.Errorf("compact did not shrink log: before=%d after=%d", before, after)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen post-compaction: the two survivors remain in order.
	s2 := mustOpen(t, path)
	defer s2.Close()
	if got := s2.Len("k"); got != 2 {
		t.Fatalf("post-compact Len = %d want 2", got)
	}
	for _, want := range []byte{'3', '4'} {
		v, ok, _ := s2.Pullp("k")
		if !ok || len(v) != 1 || v[0] != want {
			t.Errorf("post-compact Pullp = %q want %c", v, want)
		}
	}
}

func TestStoreToleratesTruncatedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.log")
	s := mustOpen(t, path)
	if err := s.Push("k", []byte("good1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Push("k", []byte("good2")); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Simulate a crash mid-write: a length prefix promising a body that is absent.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0, 0, 0, 200}); err != nil {
		t.Fatal(err)
	}
	f.Close()

	s2 := mustOpen(t, path) // must not error on the truncated tail
	defer s2.Close()
	if got := s2.Len("k"); got != 2 {
		t.Fatalf("after truncated tail Len = %d want 2", got)
	}
	for _, want := range []string{"good1", "good2"} {
		if v, _, _ := s2.Pullp("k"); string(v) != want {
			t.Errorf("Pullp = %q want %q", v, want)
		}
	}
}

func TestStoreValueIsolation(t *testing.T) {
	s := mustOpen(t, filepath.Join(t.TempDir(), "q.log"))
	defer s.Close()
	buf := []byte("frame")
	if err := s.Push("k", buf); err != nil {
		t.Fatal(err)
	}
	buf[0] = 'X' // mutate caller's buffer after Push
	if v, ok := s.Peek("k"); !ok || !bytes.Equal(v, []byte("frame")) {
		t.Errorf("Push did not copy: Peek = %q", v)
	}
	pk, _ := s.Peek("k")
	pk[0] = 'Y' // mutate the Peek copy
	if v, _, _ := s.Pullp("k"); !bytes.Equal(v, []byte("frame")) {
		t.Errorf("Peek did not copy: Pullp = %q", v)
	}
}

func TestStoreClosed(t *testing.T) {
	s := mustOpen(t, filepath.Join(t.TempDir(), "q.log"))
	s.Close()
	if err := s.Push("k", []byte("x")); !errors.Is(err, space.ErrClosed) {
		t.Errorf("Push after close = %v want ErrClosed", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.Pull(ctx, "k"); !errors.Is(err, space.ErrClosed) {
		t.Errorf("Pull after close = %v want ErrClosed", err)
	}
}

func mustOpen(t *testing.T, path string) *space.Store {
	t.Helper()
	s, err := space.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
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
