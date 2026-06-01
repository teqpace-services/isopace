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

// Package mux implements the ISO-8583 switch: it multiplexes request/response
// exchanges over a single full-duplex link.Link, correlating each response to
// its in-flight request by a caller-supplied key (typically STAN + terminal). A
// background reader dispatches frames to waiting callers; unmatched frames go to
// an optional unsolicited handler. (Named "mux" rather than "switch" because
// switch is a Go keyword and cannot be an import identifier.)
package mux

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
)

// Mux errors.
var (
	ErrTimeout      = errors.New("mux: request timed out")
	ErrClosed       = errors.New("mux: closed")
	ErrDuplicateKey = errors.New("mux: a request with this key is already in flight")
)

// Keyer extracts a correlation key from a message frame. The key for a request
// and its response must match.
type Keyer func(msg []byte) (string, error)

type config struct {
	timeout     time.Duration
	unsolicited func([]byte)
}

// Option configures a Mux.
type Option func(*config)

// WithTimeout sets the default per-request timeout (0 = none; rely on context).
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithUnsolicitedHandler sets a handler for frames that match no in-flight
// request (server-initiated requests, late responses). It runs on the reader
// goroutine, so it must not block.
func WithUnsolicitedHandler(h func([]byte)) Option {
	return func(c *config) { c.unsolicited = h }
}

// Mux correlates requests and responses over a link.
type Mux struct {
	link *link.Link
	key  Keyer
	cfg  config

	mu      sync.Mutex
	pending map[string]chan []byte

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
	readErr   atomic.Value // error from the read loop
}

// New starts a Mux over l, beginning the background read loop immediately.
func New(l *link.Link, key Keyer, opts ...Option) *Mux {
	m := &Mux{link: l, key: key, pending: make(map[string]chan []byte), done: make(chan struct{})}
	for _, o := range opts {
		o(&m.cfg)
	}
	m.wg.Add(1)
	go m.readLoop()
	return m
}

// Request sends req and waits for the response whose key matches req's key,
// honouring ctx and the configured timeout.
func (m *Mux) Request(ctx context.Context, req []byte) ([]byte, error) {
	k, err := m.key(req)
	if err != nil {
		return nil, fmt.Errorf("mux: key request: %w", err)
	}
	ch := make(chan []byte, 1)
	m.mu.Lock()
	if _, dup := m.pending[k]; dup {
		m.mu.Unlock()
		return nil, ErrDuplicateKey
	}
	m.pending[k] = ch
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.pending, k)
		m.mu.Unlock()
	}()

	if err := m.link.Send(req); err != nil {
		return nil, err
	}

	var timeout <-chan time.Time
	if m.cfg.timeout > 0 {
		t := time.NewTimer(m.cfg.timeout)
		defer t.Stop()
		timeout = t.C
	}
	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timeout:
		return nil, ErrTimeout
	case <-m.done:
		return nil, m.closedErr()
	}
}

func (m *Mux) readLoop() {
	defer m.wg.Done()
	for {
		msg, err := m.link.Receive()
		if err != nil {
			m.fail(err)
			return
		}
		k, kerr := m.key(msg)
		if kerr != nil {
			m.deliverUnsolicited(msg)
			continue
		}
		m.mu.Lock()
		ch, ok := m.pending[k]
		if ok {
			delete(m.pending, k)
		}
		m.mu.Unlock()
		if ok {
			ch <- msg // buffered(1); receiver may have already left — never blocks
		} else {
			m.deliverUnsolicited(msg)
		}
	}
}

func (m *Mux) deliverUnsolicited(msg []byte) {
	if m.cfg.unsolicited != nil {
		m.cfg.unsolicited(msg)
	}
}

func (m *Mux) fail(err error) {
	m.readErr.Store(errBox{err})
	m.closeOnce.Do(func() { close(m.done) })
}

func (m *Mux) closedErr() error {
	if v, ok := m.readErr.Load().(errBox); ok {
		return v.err
	}
	return ErrClosed
}

type errBox struct{ err error }

// Close stops the read loop, closes the link, and unblocks pending requests.
func (m *Mux) Close() error {
	m.closeOnce.Do(func() { close(m.done) })
	err := m.link.Close()
	m.wg.Wait()
	return err
}

// FieldKeyer builds a Keyer that decodes a frame with c and joins the string
// values of the given data elements (e.g. DE 11 STAN + DE 41 terminal) — the
// standard ISO-8583 request/response correlation key.
func FieldKeyer(c *iso8583.Codec, des ...int) Keyer {
	return func(msg []byte) (string, error) {
		m, err := c.Unmarshal(msg)
		if err != nil {
			return "", err
		}
		parts := make([]string, len(des))
		for i, de := range des {
			v, ok := m.Get(de)
			if !ok {
				return "", fmt.Errorf("mux: correlation DE %d absent", de)
			}
			s, err := v.String()
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return strings.Join(parts, "|"), nil
	}
}
