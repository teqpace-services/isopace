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

// Reply sends an answer to an unsolicited frame back over the same link. It is
// handed to an [WithUnsolicitedReplier] handler so a peer-initiated request — an
// echo test, a sign-off, a server-initiated advice — can be answered.
type Reply func(msg []byte) error

type config struct {
	timeout     time.Duration
	unsolicited func([]byte)
	replier     func([]byte, Reply)
	respKey     Keyer
}

// Option configures a Mux.
type Option func(*config)

// WithTimeout sets the default per-request timeout (0 = none; rely on context).
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithUnsolicitedHandler sets a handler for frames that match no in-flight
// request (server-initiated requests, late responses). It runs on the reader
// goroutine, so it must not block.
//
// Use [WithUnsolicitedReplier] instead when the handler needs to ANSWER the frame:
// a peer that echo-tests us and never receives a reply concludes the link is dead
// and tears it down.
func WithUnsolicitedHandler(h func([]byte)) Option {
	return func(c *config) { c.unsolicited = h }
}

// WithUnsolicitedReplier sets an unsolicited handler that can answer the frame, via
// the [Reply] it is given. It takes precedence over [WithUnsolicitedHandler].
//
// Like the plain handler it runs on the reader goroutine and must not block — but
// a reply is a single non-blocking write to the link, so answering inline is the
// intended use. Anything slower belongs on a queue.
func WithUnsolicitedReplier(h func(frame []byte, reply Reply)) Option {
	return func(c *config) { c.replier = h }
}

// WithResponseKeyer sets a SEPARATE keyer for inbound frames, leaving the Keyer
// passed to [New] to key outgoing requests only. Without it one function keys both
// directions, and then no keyer can both match a response to its request and tell a
// PEER's request apart from our own — the two are structurally identical.
//
// That ambiguity is not theoretical. Network-management traffic correlates on the
// trace number alone (an 0800 echo carries no terminal id), so a peer-originated
// 0800 whose trace happens to match one we have in flight is delivered to us as
// though it were the 0810 answer, and the real answer then arrives unmatched.
//
// With both keyers the caller resolves it by keying on the message type as well as
// the trace: the request keyer maps an outgoing frame to the key of the response it
// EXPECTS, and the response keyer maps an inbound frame by its OWN type. Our 0800
// registers under "0810:<trace>", the real 0810 matches it, and a peer's 0800 keys
// to "0800:<trace>" — no match, so it reaches the unsolicited handler where it
// belongs.
func WithResponseKeyer(k Keyer) Option { return func(c *config) { c.respKey = k } }

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
		k, kerr := m.responseKey(msg)
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

// responseKey keys an INBOUND frame, using the response keyer when one is set and
// otherwise the request keyer (the single-keyer behaviour).
func (m *Mux) responseKey(msg []byte) (string, error) {
	if m.cfg.respKey != nil {
		return m.cfg.respKey(msg)
	}
	return m.key(msg)
}

func (m *Mux) deliverUnsolicited(msg []byte) {
	if m.cfg.replier != nil {
		m.cfg.replier(msg, m.Send)
		return
	}
	if m.cfg.unsolicited != nil {
		m.cfg.unsolicited(msg)
	}
}

// Send writes msg to the link WITHOUT registering a correlation or waiting for a
// response. It is how a peer-initiated frame is answered — an echo test, a sign-off,
// an advice — where [Mux.Request] would be wrong: the frame we are sending IS the
// response, so there is nothing to wait for.
//
// Safe for concurrent use (the link serialises writes).
func (m *Mux) Send(msg []byte) error {
	select {
	case <-m.done:
		return m.closedErr()
	default:
	}
	return m.link.Send(msg)
}

// stop closes the mux once, recording cause for Err. The first caller wins, so a
// clean Close reports ErrClosed while a read-loop failure reports its real cause
// even if Close races in just behind it.
func (m *Mux) stop(cause error) {
	m.closeOnce.Do(func() {
		if cause != nil {
			m.readErr.Store(errBox{cause})
		}
		close(m.done)
	})
}

func (m *Mux) fail(err error) { m.stop(err) }

func (m *Mux) closedErr() error {
	if v, ok := m.readErr.Load().(errBox); ok {
		return v.err
	}
	return ErrClosed
}

type errBox struct{ err error }

// Done returns a channel closed when the mux stops — because Close was called or
// the underlying link's read loop failed. A supervisor (e.g. a reconnecting
// connector) selects on it to learn the link died without issuing a request.
func (m *Mux) Done() <-chan struct{} { return m.done }

// Err returns the cause once Done is closed: the read-loop error that downed the
// link, or ErrClosed if it was closed cleanly. It returns nil while still live.
func (m *Mux) Err() error {
	select {
	case <-m.done:
		return m.closedErr()
	default:
		return nil
	}
}

// Close stops the read loop, closes the link, and unblocks pending requests.
func (m *Mux) Close() error {
	m.stop(ErrClosed)
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
