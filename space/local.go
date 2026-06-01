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

package space

import (
	"context"
	"sync"
)

// Local is the in-process [Space] backend: a map of FIFO queues guarded by a
// mutex. Blocked In/Rd callers wait on a broadcast channel that Out and Close
// close-and-replace, so a waiter is woken without any per-call goroutine and the
// wait is fully cancelable through its context.
type Local struct {
	mu     sync.Mutex
	data   map[string][]any
	notify chan struct{}
	closed bool
}

// NewLocal returns an empty in-process space.
func NewLocal() *Local {
	return &Local{
		data:   map[string][]any{},
		notify: make(chan struct{}),
	}
}

// signal wakes all current waiters. Caller holds the lock.
func (l *Local) signal() {
	close(l.notify)
	l.notify = make(chan struct{})
}

// Out enqueues value under key and wakes any waiters.
func (l *Local) Out(key string, value any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.data[key] = append(l.data[key], value)
	l.signal()
}

// In removes and returns the head entry under key, blocking until one exists.
func (l *Local) In(ctx context.Context, key string) (any, error) {
	return l.wait(ctx, key, true)
}

// Rd returns the head entry under key without removing it, blocking until one
// exists.
func (l *Local) Rd(ctx context.Context, key string) (any, error) {
	return l.wait(ctx, key, false)
}

func (l *Local) wait(ctx context.Context, key string, take bool) (any, error) {
	for {
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			return nil, ErrClosed
		}
		if q := l.data[key]; len(q) > 0 {
			v := q[0]
			if take {
				l.pop(key, q)
			}
			l.mu.Unlock()
			return v, nil
		}
		n := l.notify
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-n:
			// Woken by Out or Close; loop and re-check.
		}
	}
}

// Inp removes and returns the head entry under key, or ok=false if none.
func (l *Local) Inp(key string) (any, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	q := l.data[key]
	if len(q) == 0 {
		return nil, false
	}
	v := q[0]
	l.pop(key, q)
	return v, true
}

// Rdp returns the head entry under key without removing it, or ok=false if none.
func (l *Local) Rdp(key string) (any, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	q := l.data[key]
	if len(q) == 0 {
		return nil, false
	}
	return q[0], true
}

// pop removes the head of q (the queue stored at key). Caller holds the lock and
// has verified len(q) > 0.
func (l *Local) pop(key string, q []any) {
	q[0] = nil // release the reference so a taken entry can be GC'd
	rest := q[1:]
	if len(rest) == 0 {
		delete(l.data, key)
	} else {
		l.data[key] = rest
	}
}

// Len reports the number of entries queued under key.
func (l *Local) Len(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.data[key])
}

// Close marks the space closed and unblocks all waiters with ErrClosed. It is
// idempotent.
func (l *Local) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	l.signal()
	return nil
}
