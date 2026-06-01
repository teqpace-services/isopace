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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Descriptor is a declarative deployment record: it names a component, the
// factory type that builds it, and an opaque per-component configuration block.
// A directory of these (one JSON object per file) is the deployable unit a
// [Deployer] watches.
type Descriptor struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config,omitempty"`
}

// Factory builds a Component from a descriptor's config block and the shared
// environment. Registered by type in a [Registry].
type Factory func(name string, config json.RawMessage, env *Env) (Component, error)

// Registry maps descriptor types to factories. It is safe for concurrent use.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty factory registry.
func NewRegistry() *Registry { return &Registry{factories: map[string]Factory{}} }

// Register binds a factory to a type name, replacing any previous binding.
func (r *Registry) Register(typ string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[typ] = f
}

// Build instantiates the component described by d.
func (r *Registry) Build(d Descriptor, env *Env) (Component, error) {
	r.mu.RLock()
	f, ok := r.factories[d.Type]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("runtime: no factory for type %q", d.Type)
	}
	c, err := f(d.Name, d.Config, env)
	if err != nil {
		return nil, fmt.Errorf("runtime: build %q (%s): %w", d.Name, d.Type, err)
	}
	return c, nil
}

// Deployer turns a directory of [Descriptor] files into running components on a
// [Host] and keeps them in sync with the directory: a new or changed file is
// (re)deployed and a removed or disabled one is undeployed. It is itself a
// Component, so a host can supervise it; while running it rescans the directory
// on an interval, giving hot (re)deploy.
type Deployer struct {
	host     *Host
	reg      *Registry
	dir      string
	interval time.Duration

	mu      sync.Mutex
	applied map[string]deployState // keyed by descriptor file path
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

type deployState struct {
	name string // deployed component name
	raw  []byte // descriptor bytes last applied
}

// DeployerOption configures a Deployer.
type DeployerOption func(*Deployer)

// WithScanInterval sets the rescan period for hot redeploy (default 5s).
func WithScanInterval(d time.Duration) DeployerOption {
	return func(dp *Deployer) { dp.interval = d }
}

// NewDeployer builds a Deployer that materialises *.json descriptors from dir
// onto host using reg.
func NewDeployer(host *Host, reg *Registry, dir string, opts ...DeployerOption) *Deployer {
	d := &Deployer{
		host:     host,
		reg:      reg,
		dir:      dir,
		interval: 5 * time.Second,
		applied:  map[string]deployState{},
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Name identifies the deployer as a component.
func (d *Deployer) Name() string { return "deployer" }

// Start begins the rescan loop, which performs an immediate first scan and then
// rescans on the configured interval. The first scan runs on the loop goroutine
// (not under Start) so that, when the deployer is itself hosted, it does not
// re-enter the host's lifecycle lock via Deploy while the host starts it.
func (d *Deployer) Start(context.Context) error {
	loopCtx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.wg.Add(1)
	go d.loop(loopCtx)
	return nil
}

// Stop ends the rescan loop. Components deployed by the deployer are owned by
// the host and are stopped by it, not here.
func (d *Deployer) Stop(context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
	return nil
}

func (d *Deployer) loop(ctx context.Context) {
	defer d.wg.Done()
	d.scanLog(ctx) // immediate first reconciliation
	t := time.NewTicker(d.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.scanLog(ctx)
		}
	}
}

func (d *Deployer) scanLog(ctx context.Context) {
	if err := d.Scan(ctx); err != nil {
		d.host.env.Log.LogAttrs(ctx, slog.LevelError, "deploy scan failed", slog.String("error", err.Error()))
	}
}

// Scan reconciles the descriptor directory with the host once: it deploys new
// and changed descriptors and undeploys removed or disabled ones. It is safe to
// call directly (e.g. in tests) as well as from the rescan loop.
func (d *Deployer) Scan(ctx context.Context) error {
	files, err := d.readDir()
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	seen := map[string]struct{}{}
	var errs []error
	// Sort for deterministic deploy order across a scan.
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		raw := files[path]
		seen[path] = struct{}{}
		prev, had := d.applied[path]
		if had && bytes.Equal(prev.raw, raw) {
			continue // unchanged
		}
		if err := d.applyOne(ctx, path, raw, prev, had); err != nil {
			errs = append(errs, err)
		}
	}

	// Undeploy descriptors whose files were removed.
	for path, st := range d.applied {
		if _, ok := seen[path]; ok {
			continue
		}
		if err := d.host.Undeploy(ctx, st.name); err != nil && !errors.Is(err, ErrNotFound) {
			errs = append(errs, fmt.Errorf("undeploy %q: %w", st.name, err))
		}
		delete(d.applied, path)
	}
	return errors.Join(errs...)
}

// applyOne (re)deploys a single descriptor file. Caller holds mu.
func (d *Deployer) applyOne(ctx context.Context, path string, raw []byte, prev deployState, had bool) error {
	var desc Descriptor
	if err := json.Unmarshal(raw, &desc); err != nil {
		return fmt.Errorf("runtime: parse descriptor %s: %w", filepath.Base(path), err)
	}
	if desc.Name == "" {
		return fmt.Errorf("runtime: descriptor %s has no name", filepath.Base(path))
	}

	// A disabled descriptor runs nothing; tear down any previous incarnation.
	if !desc.Enabled {
		if had {
			return d.teardown(ctx, path, prev)
		}
		return nil
	}

	// Build the new component BEFORE tearing down the old one, so a descriptor
	// that no longer builds (e.g. bad config) leaves the running component in
	// place rather than downing it for a replacement that never materialises.
	c, err := d.reg.Build(desc, d.host.Env())
	if err != nil {
		return err
	}
	if had {
		if err := d.teardown(ctx, path, prev); err != nil {
			return err
		}
	}
	if err := d.host.Deploy(ctx, c); err != nil {
		return err
	}
	d.applied[path] = deployState{name: desc.Name, raw: raw}
	return nil
}

// teardown undeploys a previously-applied descriptor and forgets it. Caller
// holds mu.
func (d *Deployer) teardown(ctx context.Context, path string, prev deployState) error {
	if err := d.host.Undeploy(ctx, prev.name); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("runtime: replace %q: %w", prev.name, err)
	}
	delete(d.applied, path)
	return nil
}

func (d *Deployer) readDir() (map[string][]byte, error) {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return nil, fmt.Errorf("runtime: read deploy dir: %w", err)
	}
	files := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(d.dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("runtime: read descriptor %s: %w", e.Name(), err)
		}
		files[path] = raw
	}
	return files, nil
}
