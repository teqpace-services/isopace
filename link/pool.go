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

package link

import (
	"errors"
	"sync"
	"time"
)

// Pool errors.
var (
	ErrNoHealthy  = errors.New("link: no healthy connection in pool")
	ErrPoolClosed = errors.New("link: pool closed")
)

// Pool maintains a fixed set of Links to a target, round-robining healthy ones
// and reconnecting failed slots in the background with exponential backoff.
type Pool struct {
	dial       func() (*Link, error)
	minBackoff time.Duration
	maxBackoff time.Duration

	mu    sync.Mutex
	slots []*Link
	next  int

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// PoolOption configures a Pool.
type PoolOption func(*Pool)

// WithBackoff sets the reconnect backoff bounds.
func WithBackoff(min, max time.Duration) PoolOption {
	return func(p *Pool) { p.minBackoff, p.maxBackoff = min, max }
}

// NewPool builds a pool of size connections using dial. Slots that fail to dial
// initially begin reconnecting immediately.
func NewPool(size int, dial func() (*Link, error), opts ...PoolOption) *Pool {
	p := &Pool{
		dial:       dial,
		minBackoff: 100 * time.Millisecond,
		maxBackoff: 30 * time.Second,
		slots:      make([]*Link, size),
		done:       make(chan struct{}),
	}
	for _, o := range opts {
		o(p)
	}
	for i := range p.slots {
		if l, err := dial(); err == nil {
			p.slots[i] = l
		} else {
			p.reconnect(i)
		}
	}
	return p
}

// Get returns the next healthy Link (round-robin), or ErrNoHealthy if none are
// currently connected.
func (p *Pool) Get() (*Link, error) {
	select {
	case <-p.done:
		return nil, ErrPoolClosed
	default:
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.slots)
	for i := 0; i < n; i++ {
		idx := (p.next + i) % n
		if l := p.slots[idx]; l != nil {
			p.next = (idx + 1) % n
			return l, nil
		}
	}
	return nil, ErrNoHealthy
}

// Discard removes a Link believed to be broken, closes it, and schedules its
// slot for reconnection. Callers invoke it when a Send/Receive on l fails.
func (p *Pool) Discard(l *Link) {
	if l == nil {
		return
	}
	l.Close()
	p.mu.Lock()
	for i, s := range p.slots {
		if s == l {
			p.slots[i] = nil
			p.mu.Unlock()
			p.reconnect(i)
			return
		}
	}
	p.mu.Unlock()
}

func (p *Pool) reconnect(i int) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		backoff := p.minBackoff
		for {
			select {
			case <-p.done:
				return
			default:
			}
			l, err := p.dial()
			if err == nil {
				p.mu.Lock()
				select {
				case <-p.done:
					p.mu.Unlock()
					l.Close()
					return
				default:
				}
				p.slots[i] = l
				p.mu.Unlock()
				return
			}
			t := time.NewTimer(backoff)
			select {
			case <-p.done:
				t.Stop()
				return
			case <-t.C:
			}
			if backoff *= 2; backoff > p.maxBackoff {
				backoff = p.maxBackoff
			}
		}
	}()
}

// Close shuts the pool, closing all links and stopping reconnection.
func (p *Pool) Close() error {
	p.closeOnce.Do(func() { close(p.done) })
	p.mu.Lock()
	for i, l := range p.slots {
		if l != nil {
			l.Close()
			p.slots[i] = nil
		}
	}
	p.mu.Unlock()
	p.wg.Wait()
	return nil
}
