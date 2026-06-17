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

// Package connector is the Isopace outbound switch client: a supervised,
// self-healing connection to a remote host (an acquirer, an issuer, or an
// interchange switch such as Interswitch or UP). It keeps one [link.Link] dialled
// and a [mux.Mux] running over it, automatically reconnecting with backoff when
// the link drops, so callers see a stable [Connector.Request] surface whether or
// not the underlying socket is currently up.
//
// It is the Isopace equivalent of jPOS's ChannelAdaptor + QMUX combined: the
// adaptor part owns the dial/reconnect lifecycle, the mux part multiplexes
// request/response correlation. A Connector satisfies the runtime.Component
// contract (Name/Start/Stop) without importing runtime, so a host can supervise
// it and a deployer can build it from a descriptor.
//
// Two optional hooks make it switch-agnostic: OnConnect runs after each
// (re)connect for sign-on / key exchange, and Keepalive runs on an interval for
// an echo (0800) test that holds the link open and detects a dead-but-open peer.
package connector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/mux"
)

// Connector errors.
var (
	ErrNotConnected = errors.New("connector: not connected")
	ErrClosed       = errors.New("connector: closed")
)

// Config configures a [Connector]. Name, Addr, and Keyer are required.
type Config struct {
	// Name identifies the connector (e.g. "isw", "up") in logs and to a host.
	Name string
	// Network and Addr are the dial target (default network "tcp").
	Network string
	Addr    string
	// Keyer correlates a response to its in-flight request (typically STAN +
	// terminal, via mux.FieldKeyer).
	Keyer mux.Keyer
	// LinkOptions configure the underlying link: framer (e.g. a Postilion 2-byte
	// length prefix), TLS, MAC filters, TCP keep-alive.
	LinkOptions []link.Option
	// Timeout bounds each Request (0 = rely on the caller's context).
	Timeout time.Duration
	// MinBackoff and MaxBackoff bound the reconnect backoff (defaults 250ms/30s).
	MinBackoff, MaxBackoff time.Duration
	// OnConnect runs on the freshly established mux after each (re)connect, before
	// the connector is marked up — the place for sign-on / session-key exchange. A
	// non-nil error drops the link and triggers another reconnect. Optional.
	OnConnect func(ctx context.Context, m *mux.Mux) error
	// Keepalive runs on the live mux every KeepaliveInterval — typically an echo
	// (0800) exchange. A non-nil error drops the link and forces a reconnect, so a
	// dead-but-open peer is detected without waiting for a real request. Optional.
	Keepalive         func(ctx context.Context, m *mux.Mux) error
	KeepaliveInterval time.Duration
	// Unsolicited handles frames the peer sends that match no in-flight request —
	// server-initiated advices, reversals, or sign-off. It runs on the read loop,
	// so it must not block (hand off to a queue). Optional.
	Unsolicited func(frame []byte)
	// Log receives lifecycle events (default slog.Default).
	Log *slog.Logger
}

// Connector keeps a single connection to a remote switch up and multiplexes
// requests over it, reconnecting automatically when it drops.
type Connector struct {
	cfg Config
	log *slog.Logger

	mux atomic.Pointer[mux.Mux] // current live mux; nil while disconnected

	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed atomic.Bool
}

// New validates cfg and builds a Connector. Call Start to begin connecting.
func New(cfg Config) (*Connector, error) {
	switch {
	case cfg.Name == "":
		return nil, errors.New("connector: Name required")
	case cfg.Addr == "":
		return nil, errors.New("connector: Addr required")
	case cfg.Keyer == nil:
		return nil, errors.New("connector: Keyer required")
	}
	if cfg.Network == "" {
		cfg.Network = "tcp"
	}
	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = 250 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Connector{cfg: cfg, log: cfg.Log}, nil
}

// Name identifies the connector as a component.
func (c *Connector) Name() string { return c.cfg.Name }

// Start launches the supervisor goroutine that keeps the link connected. It
// returns promptly; connection happens in the background. The supervisor runs
// until Stop, independent of the ctx passed here (which bounds Start itself).
func (c *Connector) Start(context.Context) error {
	if c.closed.Load() {
		return ErrClosed
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)
	go c.supervise(ctx)
	return nil
}

// Stop ends the supervisor and closes the current connection. After Stop the
// connector cannot be started again.
func (c *Connector) Stop(context.Context) error {
	c.closed.Store(true)
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	return nil
}

// Request sends req over the live connection and waits for the correlated
// response. It returns ErrNotConnected if the link is currently down.
func (c *Connector) Request(ctx context.Context, req []byte) ([]byte, error) {
	m := c.mux.Load()
	if m == nil {
		return nil, ErrNotConnected
	}
	return m.Request(ctx, req)
}

// Connected reports whether the link is currently up.
func (c *Connector) Connected() bool { return c.mux.Load() != nil }

// supervise dials, serves, and reconnects until ctx is cancelled.
func (c *Connector) supervise(ctx context.Context) {
	defer c.wg.Done()
	backoff := c.cfg.MinBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		m, err := c.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.log.LogAttrs(ctx, slog.LevelWarn, "connector connect failed",
				slog.String("connector", c.cfg.Name), slog.String("addr", c.cfg.Addr),
				slog.String("error", err.Error()))
			if !sleep(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff, c.cfg.MaxBackoff)
			continue
		}

		c.mux.Store(m)
		c.log.LogAttrs(ctx, slog.LevelInfo, "connector up",
			slog.String("connector", c.cfg.Name), slog.String("addr", c.cfg.Addr))
		up := time.Now()

		c.serve(ctx, m) // blocks until the link dies or ctx is cancelled
		cause := m.Err()
		c.mux.CompareAndSwap(m, nil)
		m.Close()

		if ctx.Err() != nil {
			return
		}
		c.log.LogAttrs(ctx, slog.LevelWarn, "connector down, reconnecting",
			slog.String("connector", c.cfg.Name), slog.String("cause", errString(cause)))

		// A link that stayed up a good while resets the backoff; one that flaps
		// keeps escalating it.
		if time.Since(up) >= c.cfg.MaxBackoff {
			backoff = c.cfg.MinBackoff
		}
		if !sleep(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff, c.cfg.MaxBackoff)
	}
}

// connect dials a link, starts a mux, and runs the sign-on hook. On any failure
// it tears down what it built and returns the error.
func (c *Connector) connect(ctx context.Context) (*mux.Mux, error) {
	l, err := link.DialContext(ctx, c.cfg.Network, c.cfg.Addr, c.cfg.LinkOptions...)
	if err != nil {
		return nil, err
	}
	var opts []mux.Option
	if c.cfg.Timeout > 0 {
		opts = append(opts, mux.WithTimeout(c.cfg.Timeout))
	}
	if c.cfg.Unsolicited != nil {
		opts = append(opts, mux.WithUnsolicitedHandler(c.cfg.Unsolicited))
	}
	m := mux.New(l, c.cfg.Keyer, opts...)
	if c.cfg.OnConnect != nil {
		if err := c.cfg.OnConnect(ctx, m); err != nil {
			m.Close()
			return nil, fmt.Errorf("sign-on: %w", err)
		}
	}
	return m, nil
}

// serve runs the keepalive ticker (if any) and blocks until the link dies or ctx
// is cancelled.
func (c *Connector) serve(ctx context.Context, m *mux.Mux) {
	var tick <-chan time.Time
	if c.cfg.Keepalive != nil && c.cfg.KeepaliveInterval > 0 {
		t := time.NewTicker(c.cfg.KeepaliveInterval)
		defer t.Stop()
		tick = t.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.Done():
			return
		case <-tick:
			if err := c.cfg.Keepalive(ctx, m); err != nil {
				c.log.LogAttrs(ctx, slog.LevelWarn, "connector keepalive failed",
					slog.String("connector", c.cfg.Name), slog.String("error", err.Error()))
				return // force a reconnect
			}
		}
	}
}

func nextBackoff(cur, max time.Duration) time.Duration {
	if cur *= 2; cur > max {
		return max
	}
	return cur
}

// sleep waits d, returning false if ctx is cancelled first.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
