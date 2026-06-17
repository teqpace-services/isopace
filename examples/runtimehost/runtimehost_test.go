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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/teqpace-services/isopace/runtime"
)

// recorder collects component lifecycle events in order.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(s string) {
	r.mu.Lock()
	r.events = append(r.events, s)
	r.mu.Unlock()
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// recComp is a Component that records its own start/stop, for asserting host
// lifecycle ordering deterministically.
type recComp struct {
	name string
	rec  *recorder
}

func (c *recComp) Name() string                { return c.name }
func (c *recComp) Start(context.Context) error { c.rec.add(c.name + ":start"); return nil }
func (c *recComp) Stop(context.Context) error  { c.rec.add(c.name + ":stop"); return nil }

// recFactory builds recComp values, recording into rec. A descriptor config of
// {"fail": true} forces a build error, to exercise the deployer's error path.
func recFactory(rec *recorder) runtime.Factory {
	return func(name string, cfg json.RawMessage, _ *runtime.Env) (runtime.Component, error) {
		var c struct {
			Fail bool `json:"fail"`
		}
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		if c.Fail {
			return nil, fmt.Errorf("forced build failure for %q", name)
		}
		return &recComp{name: name, rec: rec}, nil
	}
}

func quietHost() *runtime.Host {
	return runtime.New(runtime.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
}

// TestHostLifecycleOrder: components start in registration order and stop in the
// reverse order.
func TestHostLifecycleOrder(t *testing.T) {
	rec := &recorder{}
	h := quietHost()
	for _, n := range []string{"a", "b", "c"} {
		if err := h.Register(&recComp{name: n, rec: rec}); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	want := []string{"a:start", "b:start", "c:start", "c:stop", "b:stop", "a:stop"}
	if got := rec.snapshot(); !slices.Equal(got, want) {
		t.Errorf("lifecycle = %v want %v", got, want)
	}
}

// TestDeployUndeployWhileRunning: Deploy on a running host starts the component
// immediately; Undeploy stops and removes it.
func TestDeployUndeployWhileRunning(t *testing.T) {
	rec := &recorder{}
	h := quietHost()
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := h.Deploy(context.Background(), &recComp{name: "late", rec: rec}); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if got := h.Components(); !slices.Equal(got, []string{"late"}) {
		t.Errorf("components = %v want [late]", got)
	}
	if err := h.Undeploy(context.Background(), "late"); err != nil {
		t.Fatalf("Undeploy: %v", err)
	}
	if got := h.Components(); len(got) != 0 {
		t.Errorf("components = %v want empty", got)
	}
	want := []string{"late:start", "late:stop"}
	if got := rec.snapshot(); !slices.Equal(got, want) {
		t.Errorf("events = %v want %v", got, want)
	}
}

// writeDescFile writes a descriptor JSON file into dir.
func writeDescFile(t *testing.T, dir, file string, d runtime.Descriptor) {
	t.Helper()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDeployerReconciles drives the deployer's Scan directly (no timing) through
// the full reconcile lifecycle: deploy enabled, skip disabled, enable, redeploy
// on change, and undeploy on file removal.
func TestDeployerReconciles(t *testing.T) {
	rec := &recorder{}
	dir := t.TempDir()
	h := quietHost()
	reg := runtime.NewRegistry()
	reg.Register("rec", recFactory(rec))
	dep := runtime.NewDeployer(h, reg, dir)

	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx := context.Background()

	// alpha enabled, beta disabled -> only alpha deploys.
	writeDescFile(t, dir, "alpha.json", runtime.Descriptor{Name: "alpha", Type: "rec", Enabled: true})
	writeDescFile(t, dir, "beta.json", runtime.Descriptor{Name: "beta", Type: "rec", Enabled: false})
	mustScan(t, dep, ctx)
	if got := h.Components(); !slices.Equal(got, []string{"alpha"}) {
		t.Fatalf("after first scan components = %v want [alpha]", got)
	}

	// Enable beta -> beta deploys.
	writeDescFile(t, dir, "beta.json", runtime.Descriptor{Name: "beta", Type: "rec", Enabled: true})
	mustScan(t, dep, ctx)
	if got := h.Components(); !slices.Equal(got, []string{"alpha", "beta"}) {
		t.Fatalf("after enable beta components = %v want [alpha beta]", got)
	}

	// Change alpha (add a config block) -> redeploy: stop old, start new; alpha
	// moves to the end of the order.
	writeDescFile(t, dir, "alpha.json", runtime.Descriptor{Name: "alpha", Type: "rec", Enabled: true, Config: json.RawMessage(`{"fail":false}`)})
	mustScan(t, dep, ctx)
	if got := h.Components(); !slices.Equal(got, []string{"beta", "alpha"}) {
		t.Fatalf("after change alpha components = %v want [beta alpha]", got)
	}

	// Remove beta's file -> undeploy beta.
	if err := os.Remove(filepath.Join(dir, "beta.json")); err != nil {
		t.Fatal(err)
	}
	mustScan(t, dep, ctx)
	if got := h.Components(); !slices.Equal(got, []string{"alpha"}) {
		t.Fatalf("after remove beta components = %v want [alpha]", got)
	}

	want := []string{
		"alpha:start",               // first scan
		"beta:start",                // enable beta
		"alpha:stop", "alpha:start", // redeploy alpha
		"beta:stop", // remove beta
	}
	if got := rec.snapshot(); !slices.Equal(got, want) {
		t.Errorf("events = %v want %v", got, want)
	}
}

// TestDeployerBuildErrorLeavesHostClean: a descriptor that fails to build is
// reported but does not deploy, and a sibling good descriptor still deploys.
func TestDeployerBuildErrorLeavesHostClean(t *testing.T) {
	rec := &recorder{}
	dir := t.TempDir()
	h := quietHost()
	reg := runtime.NewRegistry()
	reg.Register("rec", recFactory(rec))
	dep := runtime.NewDeployer(h, reg, dir)

	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	writeDescFile(t, dir, "good.json", runtime.Descriptor{Name: "good", Type: "rec", Enabled: true})
	writeDescFile(t, dir, "bad.json", runtime.Descriptor{Name: "bad", Type: "rec", Enabled: true, Config: json.RawMessage(`{"fail":true}`)})

	if err := dep.Scan(context.Background()); err == nil {
		t.Fatal("Scan err = nil want build failure")
	}
	if got := h.Components(); !slices.Equal(got, []string{"good"}) {
		t.Errorf("components = %v want [good] (bad must not deploy)", got)
	}
}

// TestUnknownTypeIsReported: a descriptor whose type has no factory is an error.
func TestUnknownTypeIsReported(t *testing.T) {
	dir := t.TempDir()
	h := quietHost()
	dep := runtime.NewDeployer(h, runtime.NewRegistry(), dir)
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	writeDescFile(t, dir, "x.json", runtime.Descriptor{Name: "x", Type: "nope", Enabled: true})
	if err := dep.Scan(context.Background()); err == nil {
		t.Error("Scan err = nil want no-factory error")
	}
}

// TestWorkerFactoryConfig covers the example's own factory: it parses config,
// rejects bad JSON and non-positive intervals, and defaults sensibly.
func TestWorkerFactoryConfig(t *testing.T) {
	env := quietHost().Env()

	c, err := newWorker("w", json.RawMessage(`{"interval_ms":250,"label":"x"}`), env)
	if err != nil {
		t.Fatalf("newWorker: %v", err)
	}
	if c.Name() != "w" {
		t.Errorf("name = %q want w", c.Name())
	}

	if _, err := newWorker("w", json.RawMessage(`{bad`), env); err == nil {
		t.Error("bad JSON config: err = nil want error")
	}
	if _, err := newWorker("w", json.RawMessage(`{"interval_ms":0}`), env); err == nil {
		t.Error("zero interval: err = nil want error")
	}
}

func mustScan(t *testing.T, dep *runtime.Deployer, ctx context.Context) {
	t.Helper()
	if err := dep.Scan(ctx); err != nil && !errors.Is(err, runtime.ErrNotFound) {
		t.Fatalf("Scan: %v", err)
	}
}
