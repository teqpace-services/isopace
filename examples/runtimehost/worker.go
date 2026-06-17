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
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/teqpace-services/isopace/runtime"
)

// workerConfig is the config block of a "worker" descriptor.
type workerConfig struct {
	IntervalMS int    `json:"interval_ms"`
	Label      string `json:"label"`
}

// worker is a demo runtime.Component: a heartbeat emitter that runs on its own
// goroutine between Start and Stop. It stands in for any long-running managed
// service (a listener, a switch, a flow) without needing a network, so the
// example stays focused on the host's lifecycle and deploy machinery.
//
// It honours the Component contract: Start returns promptly after launching the
// goroutine, and Stop quiesces it (cancel + wait) before returning.
type worker struct {
	name     string
	label    string
	interval time.Duration
	log      *slog.Logger
	beats    runtime.Counter

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (w *worker) Name() string { return w.name }

func (w *worker) Start(context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.wg.Add(1)
	go w.run(ctx)
	return nil
}

func (w *worker) run(ctx context.Context) {
	defer w.wg.Done()
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.beats.Add(1, runtime.A("worker", w.name))
			w.log.Info("heartbeat", "worker", w.name, "label", w.label)
		}
	}
}

func (w *worker) Stop(context.Context) error {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	return nil
}

// newWorker is the runtime.Factory bound to the "worker" descriptor type. It
// reads the config block, validates it, and wires the shared env's logger and
// observer into the component — the same env every hosted component receives.
func newWorker(name string, cfg json.RawMessage, env *runtime.Env) (runtime.Component, error) {
	wc := workerConfig{IntervalMS: 1000}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &wc); err != nil {
			return nil, fmt.Errorf("worker %q: bad config: %w", name, err)
		}
	}
	if wc.IntervalMS <= 0 {
		return nil, fmt.Errorf("worker %q: interval_ms must be > 0", name)
	}
	return &worker{
		name:     name,
		label:    wc.Label,
		interval: time.Duration(wc.IntervalMS) * time.Millisecond,
		log:      env.Logger(),
		beats:    env.Observer().Counter("isopace_worker_heartbeats_total"),
	}, nil
}
