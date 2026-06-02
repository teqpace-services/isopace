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
// startup. A [Q] wraps a runtime.Host (lifecycle + declarative hot-deploy) and
// adds three Q2 staples:
//
//   - First-class switch connections: each [Q.Switch] is a self-healing
//     [connector.Connector] the host supervises, so links to interchange switches
//     (Interswitch, UP, …) are dialled, kept alive, and reconnected automatically.
//   - A name registrar: every switch and component is findable at runtime by name
//     via [Q.Get] / [Q.To], so parts of a deployment wire to each other by name.
//   - A shared tuple [space.Space] ([Q.Space]) for queue-based decoupling
//     (store-and-forward, unsolicited-message fan-out).
//
// Starting from code is a few lines:
//
//	q := teq.New()
//	isw, _ := q.Switch(connector.Config{Name: "isw", Addr: "isw.example:5000", Keyer: keyer})
//	if err := q.Start(ctx); err != nil { log.Fatal(err) }
//	resp, err := q.To("isw").Request(ctx, frame)
//	q.Stop(ctx)
//
// Or run the whole thing declaratively from a deploy directory of JSON
// descriptors — the cmd/teq daemon does exactly that and blocks until signalled.
// Switch descriptors fully describe a link (packager, framer, sign-on, echo,
// reconnect), so adding a switch needs no code.
package teq

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/teqpace-services/isopace/connector"
	"github.com/teqpace-services/isopace/runtime"
	"github.com/teqpace-services/isopace/space"
)

// Q is the container: a runtime.Host, a name registrar, and a shared space.
// Build it with [New]; it is safe for concurrent use.
type Q struct {
	host  *runtime.Host
	reg   *runtime.Registry
	log   *slog.Logger
	space space.Space

	obs             runtime.Observer
	shutdownTimeout time.Duration
	deployDir       string
	scanEvery       time.Duration

	mu      sync.RWMutex
	objects map[string]any // name registrar: switches + components by name

	deployOnce sync.Once
	stopOnce   sync.Once
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
// with [Q.Register], then [Q.Start] (or [Q.Run] / [Q.ListenAndServe]).
func New(opts ...Option) *Q {
	q := &Q{
		objects:   map[string]any{},
		space:     space.NewLocal(),
		scanEvery: 5 * time.Second,
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
	if err := q.putUnique(cfg.Name, c); err != nil {
		return nil, err
	}
	if err := q.host.Deploy(context.Background(), c); err != nil {
		q.remove(cfg.Name)
		return nil, err
	}
	return c, nil
}

// Register adds a component (a listener, an admin endpoint, a flow worker, …) to
// the host and the name registrar. It starts immediately if Q is already
// running, else when Q starts.
func (q *Q) Register(c runtime.Component) error {
	if err := q.host.Deploy(context.Background(), c); err != nil {
		return err
	}
	q.Put(c.Name(), c)
	return nil
}

// Put registers obj under name in the name registrar, replacing any prior entry.
func (q *Q) Put(name string, obj any) {
	q.mu.Lock()
	q.objects[name] = obj
	q.mu.Unlock()
}

// Get returns the object registered under name (a connector, a component, or
// anything put there), and whether it was present.
func (q *Q) Get(name string) (any, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	v, ok := q.objects[name]
	return v, ok
}

// Names lists every registered name.
func (q *Q) Names() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	names := make([]string, 0, len(q.objects))
	for n := range q.objects {
		names = append(names, n)
	}
	return names
}

// To returns the named switch connector, or nil if there is none of that name.
func (q *Q) To(name string) *connector.Connector {
	v, _ := q.Get(name)
	c, _ := v.(*connector.Connector)
	return c
}

// Switches lists the registered switch names.
func (q *Q) Switches() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	var names []string
	for n, v := range q.objects {
		if _, ok := v.(*connector.Connector); ok {
			names = append(names, n)
		}
	}
	return names
}

// Space returns the container's shared tuple space, for queue-based decoupling
// between components (store-and-forward, unsolicited fan-out).
func (q *Q) Space() space.Space { return q.space }

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

// Stop shuts down all components in reverse start order and closes the space.
func (q *Q) Stop(ctx context.Context) error {
	err := q.host.Stop(ctx)
	q.stopOnce.Do(func() { q.space.Close() })
	return err
}

// Run starts the container, blocks until ctx is cancelled, then stops it within
// the shutdown timeout.
func (q *Q) Run(ctx context.Context) error {
	if err := q.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	stopCtx, cancel := context.WithTimeout(context.Background(), q.shutdownTimeoutOr())
	defer cancel()
	return q.Stop(stopCtx)
}

// ListenAndServe runs the container until SIGINT/SIGTERM, then shuts down — the
// simplest daemon entry point, mirroring "start Q2 and let it run".
func (q *Q) ListenAndServe() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return q.Run(ctx)
}

func (q *Q) shutdownTimeoutOr() time.Duration {
	if q.shutdownTimeout > 0 {
		return q.shutdownTimeout
	}
	return 30 * time.Second
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

// putUnique registers obj under name, failing if the name is already taken.
func (q *Q) putUnique(name string, obj any) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, dup := q.objects[name]; dup {
		return fmt.Errorf("teq: %q already registered", name)
	}
	q.objects[name] = obj
	return nil
}

func (q *Q) remove(name string) {
	q.mu.Lock()
	delete(q.objects, name)
	q.mu.Unlock()
}
