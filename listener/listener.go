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

// Package listener is the server edge: it accepts TCP (optionally TLS)
// connections and hands each to a Handler as a framed link.Link, managing the
// accept loop and per-connection lifecycle with graceful shutdown.
package listener

import (
	"crypto/tls"
	"net"
	"sync"
	"sync/atomic"

	"github.com/teqpace-services/isopace/link"
)

// Handler serves one accepted connection. It returns when the connection is
// finished; the listener closes the link afterwards.
type Handler func(*link.Link)

type config struct {
	linkOpts []link.Option
	tls      *tls.Config
}

// Option configures a Listener.
type Option func(*config)

// WithLinkOptions sets the link.Options applied to each accepted connection
// (framer, filters, keep-alive).
func WithLinkOptions(o ...link.Option) Option {
	return func(c *config) { c.linkOpts = append(c.linkOpts, o...) }
}

// WithTLS terminates TLS on accepted connections using t (server certificates;
// set ClientAuth for mutual TLS).
func WithTLS(t *tls.Config) Option { return func(c *config) { c.tls = t } }

// Listener accepts framed connections.
type Listener struct {
	ln     net.Listener
	cfg    config
	mu     sync.Mutex
	conns  map[*link.Link]struct{}
	wg     sync.WaitGroup
	closed atomic.Bool
}

// Listen starts listening on network/addr.
func Listen(network, addr string, opts ...Option) (*Listener, error) {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}
	if cfg.tls != nil {
		ln = tls.NewListener(ln, cfg.tls)
	}
	return &Listener{ln: ln, cfg: cfg, conns: make(map[*link.Link]struct{})}, nil
}

// Addr returns the listening address.
func (s *Listener) Addr() net.Addr { return s.ln.Addr() }

// Serve runs the accept loop, invoking handler in its own goroutine per
// connection. It returns nil after Close, or the accept error otherwise.
func (s *Listener) Serve(handler Handler) error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if s.closed.Load() {
				return nil
			}
			return err
		}
		l := link.New(conn, s.cfg.linkOpts...)
		if !s.track(l) { // closed between Accept and track
			l.Close()
			return nil
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer s.untrack(l)
			defer l.Close()
			handler(l)
		}()
	}
}

func (s *Listener) track(l *link.Link) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return false
	}
	s.conns[l] = struct{}{}
	return true
}

func (s *Listener) untrack(l *link.Link) {
	s.mu.Lock()
	delete(s.conns, l)
	s.mu.Unlock()
}

// Close stops accepting, closes every active connection (unblocking handlers),
// and waits for all handlers to return.
func (s *Listener) Close() error {
	if s.closed.Swap(true) {
		s.wg.Wait()
		return nil
	}
	err := s.ln.Close()
	s.mu.Lock()
	for l := range s.conns {
		l.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return err
}
