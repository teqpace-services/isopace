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

// Package link is the ISO-8583 transport edge: a message-framed connection over
// TCP (optionally TLS). A Link turns a byte stream into discrete message frames
// via a pluggable Framer (length-prefix by default), applies ordered Filters on
// the way in and out, and is safe for concurrent Send from many goroutines
// while a single reader calls Receive. It carries raw message bytes; decoding is
// the caller's concern (iso8583.Codec), keeping transport and model decoupled.
package link

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Errors returned by the link layer.
var (
	ErrClosed   = errors.New("link: connection closed")
	ErrTooLarge = errors.New("link: frame exceeds maximum size")
)

// Filter transforms a message on its way out (outgoing=true) or in
// (outgoing=false) — e.g. MAC insertion/verification, logging, or padding.
type Filter interface {
	Apply(msg []byte, outgoing bool) ([]byte, error)
}

// FilterFunc adapts a function to Filter.
type FilterFunc func(msg []byte, outgoing bool) ([]byte, error)

// Apply implements Filter.
func (f FilterFunc) Apply(msg []byte, outgoing bool) ([]byte, error) { return f(msg, outgoing) }

type config struct {
	framer      Framer
	filters     []Filter
	tlsConfig   *tls.Config
	keepAlive   time.Duration
	dialTimeout time.Duration
}

// Option configures a Link.
type Option func(*config)

// WithFramer sets the message framer (default: LengthPrefix(2)).
func WithFramer(f Framer) Option { return func(c *config) { c.framer = f } }

// WithFilters appends ordered filters. On send they run in order; on receive,
// in reverse — so a filter pair wraps/unwraps symmetrically.
func WithFilters(f ...Filter) Option { return func(c *config) { c.filters = append(c.filters, f...) } }

// WithTLS enables TLS using the given config (client or server).
func WithTLS(t *tls.Config) Option { return func(c *config) { c.tlsConfig = t } }

// WithKeepAlive sets the TCP keep-alive period (0 disables).
func WithKeepAlive(d time.Duration) Option { return func(c *config) { c.keepAlive = d } }

// WithDialTimeout sets the dial timeout.
func WithDialTimeout(d time.Duration) Option { return func(c *config) { c.dialTimeout = d } }

func newConfig(opts []Option) config {
	c := config{framer: LengthPrefix(2), keepAlive: 30 * time.Second, dialTimeout: 10 * time.Second}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// Link is a message-framed connection.
type Link struct {
	conn   net.Conn
	cfg    config
	wmu    sync.Mutex // serialises concurrent Send
	closed atomic.Bool
}

// Dial opens a client Link to addr.
func Dial(network, addr string, opts ...Option) (*Link, error) {
	return DialContext(context.Background(), network, addr, opts...)
}

// DialContext opens a client Link, honouring ctx for the dial.
func DialContext(ctx context.Context, network, addr string, opts ...Option) (*Link, error) {
	cfg := newConfig(opts)
	d := &net.Dialer{Timeout: cfg.dialTimeout, KeepAlive: cfg.keepAlive}
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	if cfg.tlsConfig != nil {
		tconn := tls.Client(conn, cfg.tlsConfig)
		if err := tconn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		conn = tconn
	}
	return &Link{conn: conn, cfg: cfg}, nil
}

// New wraps an already-accepted connection (server side). For TLS servers pass a
// tls.Server connection (or use WithTLS and a *tls.Conn).
func New(conn net.Conn, opts ...Option) *Link {
	return &Link{conn: conn, cfg: newConfig(opts)}
}

// Send applies outgoing filters and writes one frame. It is safe to call from
// multiple goroutines concurrently.
func (l *Link) Send(msg []byte) error {
	if l.closed.Load() {
		return ErrClosed
	}
	out := msg
	var err error
	for _, f := range l.cfg.filters {
		if out, err = f.Apply(out, true); err != nil {
			return err
		}
	}
	l.wmu.Lock()
	defer l.wmu.Unlock()
	return l.cfg.framer.WriteFrame(l.conn, out)
}

// Receive reads one frame and applies incoming filters (in reverse order). It is
// intended to be called by a single reader goroutine.
func (l *Link) Receive() ([]byte, error) {
	msg, err := l.cfg.framer.ReadFrame(l.conn)
	if err != nil {
		return nil, err
	}
	for i := len(l.cfg.filters) - 1; i >= 0; i-- {
		if msg, err = l.cfg.filters[i].Apply(msg, false); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

// SetReadDeadline sets the deadline for future Receive calls.
func (l *Link) SetReadDeadline(t time.Time) error { return l.conn.SetReadDeadline(t) }

// Close closes the underlying connection. It is safe to call more than once.
func (l *Link) Close() error {
	if l.closed.Swap(true) {
		return nil
	}
	return l.conn.Close()
}

// RemoteAddr returns the peer address.
func (l *Link) RemoteAddr() net.Addr { return l.conn.RemoteAddr() }

// LocalAddr returns the local address.
func (l *Link) LocalAddr() net.Addr { return l.conn.LocalAddr() }
