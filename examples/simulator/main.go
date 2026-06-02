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

// Command simulator is an Isopace issuer/test host that ties the framework
// together: a runtime.Host supervises an ISO 8583 listener component and an
// admin HTTP component (health + Prometheus metrics), with the metrics registry
// wired in as the runtime observer. It is the reference for assembling a small
// switch from the building blocks.
//
//	go run ./examples/simulator -addr 127.0.0.1:8583 -admin 127.0.0.1:8584 -limit 10000
//	curl http://127.0.0.1:8584/healthz
//	curl http://127.0.0.1:8584/metrics
package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/teqpace-services/isopace/examples/posdemo"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/listener"
	"github.com/teqpace-services/isopace/ops"
	"github.com/teqpace-services/isopace/runtime"
)

const version = "0.2.0"

func main() {
	posAddr := flag.String("addr", "127.0.0.1:8583", "ISO-8583 listen address")
	adminAddr := flag.String("admin", "127.0.0.1:8584", "admin HTTP address")
	limit := flag.Int64("limit", 0, "approval limit in minor units (0 = approve all)")
	flag.Parse()

	logger := runtime.NewLogger(os.Stdout, runtime.LogOptions{Level: slog.LevelInfo})
	metrics := ops.NewMetrics()
	health := ops.NewHealth()
	health.Register("self", func(context.Context) error { return nil })

	api := ops.NewAPI(
		ops.WithHealth(health),
		ops.WithMetrics(metrics),
		ops.WithInfo(map[string]string{"service": "isopace-simulator", "version": version}),
	)

	host := runtime.New(runtime.WithLogger(logger), runtime.WithObserver(metrics))
	if err := host.Register(&issuerComponent{addr: *posAddr, issuer: posdemo.NewIssuer(*limit), metrics: metrics, log: logger}); err != nil {
		logger.Error("register issuer", "err", err)
		os.Exit(1)
	}
	if err := host.Register(&adminComponent{addr: *adminAddr, handler: api.Handler()}); err != nil {
		logger.Error("register admin", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("simulator starting", "iso8583", *posAddr, "admin", *adminAddr, "version", version)
	if err := host.Run(ctx); err != nil {
		logger.Error("host run failed", "err", err)
		os.Exit(1)
	}
	logger.Info("simulator stopped")
}

// issuerComponent serves the ISO-8583 listener and records request metrics.
type issuerComponent struct {
	addr    string
	issuer  *posdemo.Issuer
	metrics *ops.Metrics
	log     *slog.Logger
	srv     *listener.Listener
}

func (c *issuerComponent) Name() string { return "issuer" }

func (c *issuerComponent) Start(context.Context) error {
	srv, err := listener.Listen("tcp", c.addr)
	if err != nil {
		return err
	}
	c.srv = srv
	go func() {
		if err := srv.Serve(c.handle); err != nil {
			c.log.Error("issuer serve stopped", "err", err)
		}
	}()
	return nil
}

func (c *issuerComponent) handle(l *link.Link) {
	for {
		req, err := l.Receive()
		if err != nil {
			return
		}
		c.metrics.Counter("isopace_requests_total").Add(1)
		_, span := c.metrics.StartSpan(context.Background(), "authorize")
		resp, err := c.issuer.Respond(req)
		span.End()
		if err != nil {
			c.metrics.Counter("isopace_errors_total").Add(1)
			return
		}
		if err := l.Send(resp); err != nil {
			return
		}
	}
}

func (c *issuerComponent) Stop(context.Context) error {
	if c.srv == nil {
		return nil
	}
	return c.srv.Close()
}

// adminComponent serves the ops admin HTTP API.
type adminComponent struct {
	addr    string
	handler http.Handler
	srv     *http.Server
}

func (c *adminComponent) Name() string { return "admin" }

func (c *adminComponent) Start(context.Context) error {
	ln, err := net.Listen("tcp", c.addr)
	if err != nil {
		return err
	}
	c.srv = &http.Server{Handler: c.handler}
	go func() { _ = c.srv.Serve(ln) }()
	return nil
}

func (c *adminComponent) Stop(ctx context.Context) error {
	if c.srv == nil {
		return nil
	}
	return c.srv.Shutdown(ctx)
}
