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

// Package teq is the Isopace container — the jPOS Q2 analog — assembled for easy
// startup. A [Q] wraps a runtime.Host (lifecycle + declarative deploy) and adds
// first-class switch connections: each [Q.Switch] registers a self-healing
// [connector.Connector] the host supervises, so links to interchange switches
// (Interswitch, UP, …) are dialled, kept alive, and reconnected automatically.
//
// Starting from code is a few lines:
//
//	q := teq.New()
//	isw, _ := q.Switch(connector.Config{Name: "isw", Addr: "isw.example:5000", Keyer: keyer})
//	up,  _ := q.Switch(connector.Config{Name: "up",  Addr: "up.example:6000",  Keyer: keyer})
//	if err := q.Start(ctx); err != nil { log.Fatal(err) }
//	resp, err := isw.Request(ctx, frame)   // or q.To("isw").Request(ctx, frame)
//	...
//	q.Stop(ctx)
//
// Or run it as a daemon that shuts down on SIGINT/SIGTERM with q.ListenAndServe().
// Switches can also be declared as JSON descriptors in a deploy directory (see
// [WithDeployDir]); the host hot-(re)deploys them like any other component.
package teq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/teqpace-services/isopace/connector"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/mux"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/runtime"
)

// defaultCodec backs the keyer for declaratively-deployed switches (the code path
// can supply any Keyer). ISO 8583:1987 variant A is the neutral default.
var defaultCodec = iso8583.NewCodec(packager.ISO87A())

// Q is the container: a runtime.Host plus a registry of named switch connectors.
// Build it with [New]; it is safe for concurrent use.
type Q struct {
	host *runtime.Host
	reg  *runtime.Registry
	log  *slog.Logger

	obs             runtime.Observer
	shutdownTimeout time.Duration
	deployDir       string
	scanEvery       time.Duration

	mu         sync.RWMutex
	connectors map[string]*connector.Connector

	deployOnce sync.Once
}

// Option configures a [Q].
type Option func(*Q)

// WithLogger sets the container logger (default a text logger to stdout at info).
func WithLogger(l *slog.Logger) Option { return func(q *Q) { q.log = l } }

// WithObserver sets the metrics/traces facade shared with components.
func WithObserver(o runtime.Observer) Option { return func(q *Q) { q.obs = o } }

// WithShutdownTimeout bounds how long Run/ListenAndServe wait for shutdown.
func WithShutdownTimeout(d time.Duration) Option { return func(q *Q) { q.shutdownTimeout = d } }

// WithDeployDir makes the container watch dir for JSON component descriptors and
// hot-(re)deploy them, including switch connectors (type "connector").
func WithDeployDir(dir string) Option { return func(q *Q) { q.deployDir = dir } }

// WithScanInterval sets the deploy-dir rescan period (default 5s).
func WithScanInterval(d time.Duration) Option { return func(q *Q) { q.scanEvery = d } }

// New builds a container. Register switches with [Q.Switch] and other components
// with [Q.Register], then [Q.Start] (or [Q.Run]/[Q.ListenAndServe]).
func New(opts ...Option) *Q {
	q := &Q{
		connectors: map[string]*connector.Connector{},
		scanEvery:  5 * time.Second,
	}
	for _, o := range opts {
		o(q)
	}
	if q.log == nil {
		q.log = runtime.NewLogger(os.Stdout, runtime.LogOptions{Level: slog.LevelInfo})
	}
	hostOpts := []runtime.Option{runtime.WithLogger(q.log)}
	if q.obs != nil {
		hostOpts = append(hostOpts, runtime.WithObserver(q.obs))
	}
	if q.shutdownTimeout > 0 {
		hostOpts = append(hostOpts, runtime.WithShutdownTimeout(q.shutdownTimeout))
	}
	q.host = runtime.New(hostOpts...)
	q.reg = runtime.NewRegistry()
	q.reg.Register("connector", q.connectorFactory)
	return q
}

// Switch registers a switch connection by name and returns its connector. The
// host supervises it: if Q is already running it starts connecting immediately,
// otherwise it starts when Q does. The connector reconnects automatically.
func (q *Q) Switch(cfg connector.Config) (*connector.Connector, error) {
	if cfg.Log == nil {
		cfg.Log = q.log
	}
	c, err := connector.New(cfg)
	if err != nil {
		return nil, err
	}
	q.mu.Lock()
	if _, dup := q.connectors[cfg.Name]; dup {
		q.mu.Unlock()
		return nil, fmt.Errorf("teq: switch %q already registered", cfg.Name)
	}
	q.connectors[cfg.Name] = c
	q.mu.Unlock()

	if err := q.host.Deploy(context.Background(), c); err != nil {
		q.mu.Lock()
		delete(q.connectors, cfg.Name)
		q.mu.Unlock()
		return nil, err
	}
	return c, nil
}

// To returns the named switch connector, or nil if there is none.
func (q *Q) To(name string) *connector.Connector {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.connectors[name]
}

// Switches lists the registered switch names.
func (q *Q) Switches() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	names := make([]string, 0, len(q.connectors))
	for n := range q.connectors {
		names = append(names, n)
	}
	return names
}

// Register adds a component (a listener, an admin endpoint, a flow worker, …) to
// the host. It starts immediately if Q is already running, else when Q starts.
func (q *Q) Register(c runtime.Component) error {
	return q.host.Deploy(context.Background(), c)
}

// Host exposes the underlying runtime.Host for advanced supervision.
func (q *Q) Host() *runtime.Host { return q.host }

// Registry exposes the descriptor factory registry, so callers can register
// custom component types for the deploy directory.
func (q *Q) Registry() *runtime.Registry { return q.reg }

// Logger returns the container logger.
func (q *Q) Logger() *slog.Logger { return q.log }

// Start brings up every registered component (and the deploy-dir watcher, if
// configured). Switch connectors begin dialling in the background.
func (q *Q) Start(ctx context.Context) error {
	if err := q.ensureDeployer(); err != nil {
		return err
	}
	return q.host.Start(ctx)
}

// Stop shuts down all components in reverse start order.
func (q *Q) Stop(ctx context.Context) error { return q.host.Stop(ctx) }

// Run starts the container, blocks until ctx is cancelled, then stops it within
// the shutdown timeout.
func (q *Q) Run(ctx context.Context) error {
	if err := q.ensureDeployer(); err != nil {
		return err
	}
	return q.host.Run(ctx)
}

// ListenAndServe runs the container until SIGINT/SIGTERM, then shuts down — the
// simplest daemon entry point, mirroring "start Q2 and let it run".
func (q *Q) ListenAndServe() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return q.Run(ctx)
}

// ensureDeployer registers the deploy-dir watcher exactly once, before start.
func (q *Q) ensureDeployer() error {
	if q.deployDir == "" {
		return nil
	}
	var err error
	q.deployOnce.Do(func() {
		dep := runtime.NewDeployer(q.host, q.reg, q.deployDir, runtime.WithScanInterval(q.scanEvery))
		err = q.host.Register(dep)
	})
	return err
}

// connectorDesc is the JSON config block of a "connector" descriptor.
type connectorDesc struct {
	Addr           string `json:"addr"`
	Network        string `json:"network"`
	TimeoutMS      int    `json:"timeout_ms"`
	MinBackoffMS   int    `json:"min_backoff_ms"`
	MaxBackoffMS   int    `json:"max_backoff_ms"`
	CorrelationDEs []int  `json:"correlation_des"` // default [11, 41]
}

// connectorFactory builds a switch connector from a descriptor. It uses the
// default ISO87A codec for response correlation; switches needing a custom
// framer/profile/keyer should be registered via [Q.Switch] in code instead.
func (q *Q) connectorFactory(name string, cfg json.RawMessage, env *runtime.Env) (runtime.Component, error) {
	d := connectorDesc{Network: "tcp", CorrelationDEs: []int{11, 41}}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &d); err != nil {
			return nil, fmt.Errorf("teq: connector %q config: %w", name, err)
		}
	}
	if d.Addr == "" {
		return nil, fmt.Errorf("teq: connector %q: addr required", name)
	}
	c, err := connector.New(connector.Config{
		Name:       name,
		Network:    d.Network,
		Addr:       d.Addr,
		Keyer:      mux.FieldKeyer(defaultCodec, d.CorrelationDEs...),
		Timeout:    msDur(d.TimeoutMS),
		MinBackoff: msDur(d.MinBackoffMS),
		MaxBackoff: msDur(d.MaxBackoffMS),
		Log:        env.Logger(),
	})
	if err != nil {
		return nil, err
	}
	q.mu.Lock()
	q.connectors[name] = c
	q.mu.Unlock()
	return c, nil
}

func msDur(ms int) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
