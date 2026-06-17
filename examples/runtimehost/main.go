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

// Command runtimehost demonstrates runtime.Host — the Isopace component
// container (the jPOS Q2 analog). A Host supervises a set of Components with a
// start/stop lifecycle (started in registration order, stopped in reverse), and
// a Deployer turns a directory of declarative JSON descriptors into running
// components, hot-(re)deploying them as the files change. This example scripts
// that story end to end against a temporary deploy directory; no network needed.
//
//	go run ./examples/runtimehost
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/teqpace-services/isopace/runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "runtimehost:", err)
		os.Exit(1)
	}
}

func run() error {
	logger := runtime.NewLogger(os.Stdout, runtime.LogOptions{Level: slog.LevelInfo})

	// A scratch deploy directory the Deployer will watch.
	dir, err := os.MkdirTemp("", "isopace-deploy-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// The host shares its logger + observer with every component via Env.
	host := runtime.New(
		runtime.WithLogger(logger),
		runtime.WithObserver(runtime.SlogObserver{Log: logger}),
	)

	// A factory registry maps a descriptor "type" to the code that builds it.
	reg := runtime.NewRegistry()
	reg.Register("worker", newWorker)

	// The Deployer is itself a Component: the host supervises it, and while it
	// runs it rescans the directory on its interval for hot (re)deploy.
	dep := runtime.NewDeployer(host, reg, dir, runtime.WithScanInterval(150*time.Millisecond))

	// One hand-registered (static) component plus the deployer. Both are managed
	// by the host; the static one shows the Register path, the deployer shows the
	// declarative path.
	clock := &worker{
		name:     "clock",
		label:    "static",
		interval: 500 * time.Millisecond,
		log:      logger,
		beats:    host.Env().Observer().Counter("isopace_worker_heartbeats_total"),
	}
	if err := host.Register(clock); err != nil {
		return err
	}
	if err := host.Register(dep); err != nil {
		return err
	}

	step := func(title string) {
		fmt.Printf("\n>> %s\n   components: %v\n", title, host.Components())
	}

	ctx := context.Background()
	fmt.Println("== Isopace runtime.Host demo: component container (Q2-style) ==")

	// 1. Start: the static clock and the deployer come up, in registration order.
	if err := host.Start(ctx); err != nil {
		return err
	}
	step("started host (deploy dir empty)")

	// 2. Drop two descriptors in: alpha enabled, beta disabled. The deployer's
	//    next scan deploys alpha and ignores beta.
	writeDesc(dir, "alpha.json", "alpha", true, 200, "primary")
	writeDesc(dir, "beta.json", "beta", false, 300, "secondary")
	settle()
	step("dropped alpha (enabled) + beta (disabled)")

	// 3. Enable beta by rewriting its descriptor.
	writeDesc(dir, "beta.json", "beta", true, 300, "secondary")
	settle()
	step("enabled beta")

	// 4. Change alpha's config: the deployer redeploys it (stop old, start new),
	//    so alpha moves to the end of the lifecycle order.
	writeDesc(dir, "alpha.json", "alpha", true, 500, "primary-slowed")
	settle()
	step("changed alpha's interval -> hot redeploy")

	// 5. Remove beta's descriptor file: the deployer undeploys it.
	if err := os.Remove(filepath.Join(dir, "beta.json")); err != nil {
		return err
	}
	settle()
	step("removed beta's descriptor -> undeploy")

	// 6. Stop: every component is stopped in reverse registration order.
	fmt.Println("\n>> stopping host (components stop in reverse order)")
	return host.Stop(ctx)
}

// settle waits a few deployer scan intervals so a file change is reconciled
// before the demo prints the resulting component set.
func settle() { time.Sleep(450 * time.Millisecond) }

// writeDesc writes a worker descriptor file into the deploy directory.
func writeDesc(dir, file, name string, enabled bool, intervalMS int, label string) {
	cfg, _ := json.Marshal(workerConfig{IntervalMS: intervalMS, Label: label})
	raw, _ := json.MarshalIndent(runtime.Descriptor{
		Name:    name,
		Type:    "worker",
		Enabled: enabled,
		Config:  cfg,
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, file), raw, 0o600); err != nil {
		panic(err)
	}
}
