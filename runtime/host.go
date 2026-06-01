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

package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Host errors.
var (
	ErrDuplicate = errors.New("runtime: component already registered")
	ErrNotFound  = errors.New("runtime: component not found")
	ErrRunning   = errors.New("runtime: host already started")
)

// Host owns the lifecycle of a set of [Component] values. It starts them in
// registration order and stops them in the reverse order, so a component is
// always started after — and stopped before — the components it depends on.
// Components may also be added and removed while the host runs via [Host.Deploy]
// and [Host.Undeploy].
//
// All lifecycle methods are serialised on a single lock; a Component's Start or
// Stop must not call back into its Host *synchronously* (within the Start/Stop
// call itself), which would re-enter the non-reentrant lock. A component may
// freely interact with the host from its own background goroutines after Start
// returns — that is ordinary lock contention, not re-entrancy, and is how the
// [Deployer] drives Deploy/Undeploy.
type Host struct {
	env             *Env
	shutdownTimeout time.Duration

	mu      sync.Mutex
	order   []string
	comps   map[string]Component
	running map[string]struct{}
	started bool
}

// Option configures a Host.
type Option func(*Host)

// WithLogger sets the host (and component env) logger.
func WithLogger(l *slog.Logger) Option { return func(h *Host) { h.env.Log = l } }

// WithObserver sets the observability facade shared with components.
func WithObserver(o Observer) Option { return func(h *Host) { h.env.Obs = o } }

// WithConfig sets the root configuration shared with components.
func WithConfig(c *Config) Option { return func(h *Host) { h.env.Config = c } }

// WithShutdownTimeout bounds how long Run waits for Stop to complete.
func WithShutdownTimeout(d time.Duration) Option {
	return func(h *Host) { h.shutdownTimeout = d }
}

// New builds a Host. Defaults: slog.Default logger, no-op observer, empty
// config, 30s shutdown timeout.
func New(opts ...Option) *Host {
	h := &Host{
		env:             &Env{Log: slog.Default(), Obs: NopObserver{}},
		shutdownTimeout: 30 * time.Second,
		comps:           map[string]Component{},
		running:         map[string]struct{}{},
	}
	for _, o := range opts {
		o(h)
	}
	if h.env.Log == nil {
		h.env.Log = slog.Default()
	}
	if h.env.Obs == nil {
		h.env.Obs = NopObserver{}
	}
	return h
}

// Env returns the shared environment handed to component factories.
func (h *Host) Env() *Env { return h.env }

// Register adds a component before the host starts. After Start, use Deploy.
func (h *Host) Register(c Component) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return ErrRunning
	}
	return h.add(c)
}

// add inserts c into the registry. Caller holds mu.
func (h *Host) add(c Component) error {
	name := c.Name()
	if _, dup := h.comps[name]; dup {
		return fmt.Errorf("%w: %q", ErrDuplicate, name)
	}
	h.comps[name] = c
	h.order = append(h.order, name)
	return nil
}

// Start starts every registered component in registration order. If one fails,
// the components already started are stopped in reverse and the error returned.
func (h *Host) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return ErrRunning
	}
	for _, name := range h.order {
		if err := h.startOne(ctx, name); err != nil {
			startErr := fmt.Errorf("runtime: start %q: %w", name, err)
			if unwindErr := h.stopStarted(ctx); unwindErr != nil {
				return errors.Join(startErr, unwindErr)
			}
			return startErr
		}
	}
	h.started = true
	return nil
}

// startOne starts a single component and records it running. Caller holds mu.
func (h *Host) startOne(ctx context.Context, name string) error {
	c := h.comps[name]
	h.env.Log.LogAttrs(ctx, slog.LevelInfo, "component starting", slog.String("component", name))
	if err := c.Start(ctx); err != nil {
		return err
	}
	h.running[name] = struct{}{}
	return nil
}

// stopStarted stops running components in reverse registration order, used to
// unwind a partial Start. It returns the joined stop errors so Start can surface
// them alongside the original start failure. Caller holds mu.
func (h *Host) stopStarted(ctx context.Context) error {
	var errs []error
	for i := len(h.order) - 1; i >= 0; i-- {
		name := h.order[i]
		if _, ok := h.running[name]; !ok {
			continue
		}
		if err := h.stopOne(ctx, name); err != nil {
			errs = append(errs, fmt.Errorf("unwind stop %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// stopOne stops a single component and clears its running mark. Caller holds mu.
func (h *Host) stopOne(ctx context.Context, name string) error {
	c := h.comps[name]
	h.env.Log.LogAttrs(ctx, slog.LevelInfo, "component stopping", slog.String("component", name))
	err := c.Stop(ctx)
	delete(h.running, name)
	if err != nil {
		h.env.Log.LogAttrs(ctx, slog.LevelError, "component stop failed",
			slog.String("component", name), slog.String("error", err.Error()))
	}
	return err
}

// Stop stops all running components in reverse registration order, joining any
// stop errors. The host can be started again afterwards.
func (h *Host) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var errs []error
	for i := len(h.order) - 1; i >= 0; i-- {
		name := h.order[i]
		if _, ok := h.running[name]; !ok {
			continue
		}
		if err := h.stopOne(ctx, name); err != nil {
			errs = append(errs, fmt.Errorf("stop %q: %w", name, err))
		}
	}
	h.started = false
	return errors.Join(errs...)
}

// Deploy adds a component and, if the host is already running, starts it
// immediately. Before Start it behaves like Register.
func (h *Host) Deploy(ctx context.Context, c Component) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.add(c); err != nil {
		return err
	}
	if h.started {
		if err := h.startOne(ctx, c.Name()); err != nil {
			// Roll the failed component back out of the registry.
			h.remove(c.Name())
			return fmt.Errorf("runtime: deploy %q: %w", c.Name(), err)
		}
	}
	return nil
}

// Undeploy stops a component (if running) and removes it from the registry.
func (h *Host) Undeploy(ctx context.Context, name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.comps[name]; !ok {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	var err error
	if _, ok := h.running[name]; ok {
		err = h.stopOne(ctx, name)
	}
	h.remove(name)
	return err
}

// remove deletes name from the registry and order. Caller holds mu.
func (h *Host) remove(name string) {
	delete(h.comps, name)
	delete(h.running, name)
	for i, n := range h.order {
		if n == name {
			h.order = append(h.order[:i], h.order[i+1:]...)
			break
		}
	}
}

// Components returns the registered component names in registration order.
func (h *Host) Components() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.order...)
}

// Run starts the host, blocks until ctx is cancelled, then stops it within the
// shutdown timeout. Pass a context from signal.NotifyContext for signal-driven
// shutdown. The returned error joins a start error (if any) with the stop error.
func (h *Host) Run(ctx context.Context) error {
	if err := h.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	stopCtx, cancel := context.WithTimeout(context.Background(), h.shutdownTimeout)
	defer cancel()
	return h.Stop(stopCtx)
}
