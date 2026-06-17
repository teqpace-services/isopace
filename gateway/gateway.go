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

// Package gateway is the Isopace inbound switch server: it listens for ISO-8583
// connections, and for each received message routes it to an upstream
// switch/processor, optionally transforming the request on the way out and the
// response on the way back, then replies on the originating link. It is the
// store-and-forward heart of a switch — the inbound counterpart to
// connector.Connector (outbound).
//
// Per message the gateway: decodes the inbound frame, calls [Config.Route] to
// pick the destination [Forwarder], runs [Config.BeforeRequest] to edit the
// request, forwards via the destination, runs [Config.BeforeResponse] to edit the
// response, and sends it back. Messages on one link are handled concurrently, so
// responses return as soon as the upstream answers. A Gateway satisfies the
// runtime.Component contract (Name/Start/Stop) so a host can supervise it.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/listener"
	"github.com/teqpace-services/isopace/trace"
)

// ErrNoRoute is returned (and passed to OnError) when Route selects no destination.
var ErrNoRoute = errors.New("gateway: no route for request")

// Forwarder sends a request frame upstream and returns the response frame.
// *connector.Connector satisfies this, so a gateway routes to a named switch.
type Forwarder interface {
	Request(ctx context.Context, req []byte) ([]byte, error)
}

// Config configures a [Gateway]. Name, Addr, Codec, and Route are required.
type Config struct {
	// Name identifies the gateway in logs and to a host.
	Name string
	// Network and Addr are the inbound listen target (default network "tcp").
	Network string
	Addr    string
	// Codec decodes inbound frames and encodes outbound ones. It is the profile
	// the inbound peers speak; a destination on a different profile is the
	// caller's concern inside BeforeRequest (re-encode) or a per-route codec.
	Codec *iso8583.Codec
	// LinkOptions configure accepted connections: framer, TLS, MAC filters.
	LinkOptions []link.Option

	// Route picks the upstream for a decoded inbound request. Returning a nil
	// Forwarder (or ErrNoRoute) sends the request down the OnError path.
	Route func(ctx context.Context, req *iso8583.Message) (Forwarder, error)
	// BeforeRequest may edit the request before it is forwarded. It receives a
	// mutable clone of the inbound message; return the message to send (returning
	// nil forwards the clone unchanged). Optional.
	BeforeRequest func(ctx context.Context, req *iso8583.Message) (*iso8583.Message, error)
	// BeforeResponse may edit the response before it is returned to the client. It
	// receives the original inbound request and the upstream response. Optional.
	BeforeResponse func(ctx context.Context, req, resp *iso8583.Message) (*iso8583.Message, error)
	// OnError builds the reply when routing or forwarding fails (e.g. a 0210 with
	// response code 91). If nil, the inbound message is dropped with no reply.
	OnError func(ctx context.Context, req *iso8583.Message, cause error) (*iso8583.Message, error)

	// Timeout bounds handling of one message (route + forward + transform).
	Timeout time.Duration
	// Trace, if set, receives a completed trace.Trace for every handled message —
	// the full request/response lifecycle (steps + masked message dumps) as one
	// unit. The trace is carried in the per-message context, so Route /
	// BeforeRequest / BeforeResponse annotate the same trace via trace.From(ctx).
	Trace func(*trace.Trace)
	// Log receives lifecycle and per-message error events (default slog.Default).
	Log *slog.Logger
}

// Gateway is a running inbound switch server.
type Gateway struct {
	cfg Config
	log *slog.Logger
	srv *listener.Listener
	seq atomic.Uint64 // per-request trace id sequence
}

// New validates cfg and builds a Gateway. Call Start to begin listening.
func New(cfg Config) (*Gateway, error) {
	switch {
	case cfg.Name == "":
		return nil, errors.New("gateway: Name required")
	case cfg.Addr == "":
		return nil, errors.New("gateway: Addr required")
	case cfg.Codec == nil:
		return nil, errors.New("gateway: Codec required")
	case cfg.Route == nil:
		return nil, errors.New("gateway: Route required")
	}
	if cfg.Network == "" {
		cfg.Network = "tcp"
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Gateway{cfg: cfg, log: cfg.Log}, nil
}

// Name identifies the gateway as a component.
func (g *Gateway) Name() string { return g.cfg.Name }

// Start opens the listener and serves accepted connections in the background.
func (g *Gateway) Start(context.Context) error {
	var lopts []listener.Option
	if len(g.cfg.LinkOptions) > 0 {
		lopts = append(lopts, listener.WithLinkOptions(g.cfg.LinkOptions...))
	}
	srv, err := listener.Listen(g.cfg.Network, g.cfg.Addr, lopts...)
	if err != nil {
		return err
	}
	g.srv = srv
	go func() {
		if err := srv.Serve(g.serveConn); err != nil {
			g.log.LogAttrs(context.Background(), slog.LevelError, "gateway serve stopped",
				slog.String("gateway", g.cfg.Name), slog.String("error", err.Error()))
		}
	}()
	g.log.LogAttrs(context.Background(), slog.LevelInfo, "gateway listening",
		slog.String("gateway", g.cfg.Name), slog.String("addr", g.Addr()))
	return nil
}

// Stop closes the listener and all live connections.
func (g *Gateway) Stop(context.Context) error {
	if g.srv == nil {
		return nil
	}
	return g.srv.Close()
}

// Addr returns the actual listen address (resolved after Start).
func (g *Gateway) Addr() string {
	if g.srv == nil {
		return g.cfg.Addr
	}
	return g.srv.Addr().String()
}

// serveConn reads frames off one connection and handles each concurrently.
func (g *Gateway) serveConn(l *link.Link) {
	for {
		frame, err := l.Receive()
		if err != nil {
			return
		}
		go g.handle(l, frame)
	}
}

// handle processes one inbound frame and writes the reply back on l.
func (g *Gateway) handle(l *link.Link, frame []byte) {
	ctx := context.Background()
	if g.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.cfg.Timeout)
		defer cancel()
	}

	// Per-request trace, carried in ctx so hooks annotate the same one.
	var tr *trace.Trace
	if g.cfg.Trace != nil {
		tr = trace.New(fmt.Sprintf("%s-%d", g.cfg.Name, g.seq.Add(1)))
		ctx = trace.With(ctx, tr)
		tr.Step("received", "from", l.RemoteAddr(), "bytes", len(frame))
		defer g.cfg.Trace(tr)
	}

	req, err := g.cfg.Codec.Unmarshal(frame)
	if err != nil {
		tr.Fail(fmt.Errorf("decode inbound: %w", err))
		g.log.LogAttrs(ctx, slog.LevelWarn, "gateway: undecodable inbound frame",
			slog.String("gateway", g.cfg.Name), slog.String("error", err.Error()))
		return
	}
	tr.Message("request", req)

	resp, err := g.process(ctx, req)
	if err != nil {
		tr.Fail(err)
		if resp = g.errorReply(ctx, req, err); resp == nil {
			return
		}
	}

	out, err := g.cfg.Codec.Marshal(resp, nil)
	if err != nil {
		tr.Fail(fmt.Errorf("encode response: %w", err))
		g.log.LogAttrs(ctx, slog.LevelError, "gateway: encode response",
			slog.String("gateway", g.cfg.Name), slog.String("error", err.Error()))
		return
	}
	tr.Message("response", resp)
	if err := l.Send(out); err != nil {
		tr.Fail(fmt.Errorf("send response: %w", err))
		g.log.LogAttrs(ctx, slog.LevelWarn, "gateway: send response",
			slog.String("gateway", g.cfg.Name), slog.String("error", err.Error()))
		return
	}
	tr.Step("replied", "bytes", len(out))
}

// process routes, transforms, forwards, and transforms the response, recording
// each stage on the context's trace (if any).
func (g *Gateway) process(ctx context.Context, req *iso8583.Message) (*iso8583.Message, error) {
	tr := trace.From(ctx)

	dest, err := g.cfg.Route(ctx, req)
	if err != nil {
		return nil, err
	}
	if dest == nil {
		return nil, ErrNoRoute
	}
	tr.Step("route", "dest", forwarderName(dest))

	out := req
	if g.cfg.BeforeRequest != nil {
		edited, err := g.cfg.BeforeRequest(ctx, req.Clone())
		if err != nil {
			return nil, fmt.Errorf("before-request: %w", err)
		}
		if edited != nil {
			out = edited
			tr.Message("forwarded", out)
		}
	}

	reqFrame, err := g.cfg.Codec.Marshal(out, nil)
	if err != nil {
		return nil, fmt.Errorf("encode forward: %w", err)
	}
	started := time.Now()
	respFrame, err := dest.Request(ctx, reqFrame)
	if err != nil {
		return nil, err
	}
	tr.Step("forwarded", "dur", time.Since(started).Round(time.Microsecond))
	resp, err := g.cfg.Codec.Unmarshal(respFrame)
	if err != nil {
		return nil, fmt.Errorf("decode upstream response: %w", err)
	}

	if g.cfg.BeforeResponse != nil {
		edited, err := g.cfg.BeforeResponse(ctx, req, resp)
		if err != nil {
			return nil, fmt.Errorf("before-response: %w", err)
		}
		if edited != nil {
			resp = edited
		}
	}
	return resp, nil
}

// forwarderName returns a Forwarder's Name (if it has one) for trace detail.
func forwarderName(f Forwarder) string {
	if n, ok := f.(interface{ Name() string }); ok {
		return n.Name()
	}
	return fmt.Sprintf("%T", f)
}

// errorReply logs the failure and builds the reply via OnError, if configured.
func (g *Gateway) errorReply(ctx context.Context, req *iso8583.Message, cause error) *iso8583.Message {
	g.log.LogAttrs(ctx, slog.LevelWarn, "gateway: routing/forward failed",
		slog.String("gateway", g.cfg.Name), slog.String("error", cause.Error()))
	if g.cfg.OnError == nil {
		return nil
	}
	resp, err := g.cfg.OnError(ctx, req, cause)
	if err != nil {
		g.log.LogAttrs(ctx, slog.LevelError, "gateway: on-error reply failed",
			slog.String("gateway", g.cfg.Name), slog.String("error", err.Error()))
		return nil
	}
	return resp
}
