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
	"log/slog"
	"os"
	"sync"
	"time"
)

// ConfigWatcher reloads a [Config] when its backing file changes and invokes an
// optional callback with the refreshed config. It is a [Component], so a [Host]
// can supervise hot configuration reload. Change detection polls the file's
// modification time and size, which is portable and dependency-free.
type ConfigWatcher struct {
	cfg      *Config
	interval time.Duration
	onReload func(*Config)
	log      *slog.Logger

	last   fileStamp
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type fileStamp struct {
	mod  time.Time
	size int64
	ok   bool
}

// WatcherOption configures a ConfigWatcher.
type WatcherOption func(*ConfigWatcher)

// WithWatchInterval sets the poll period (default 2s).
func WithWatchInterval(d time.Duration) WatcherOption {
	return func(w *ConfigWatcher) { w.interval = d }
}

// OnReload registers a callback invoked after each successful reload.
func OnReload(fn func(*Config)) WatcherOption {
	return func(w *ConfigWatcher) { w.onReload = fn }
}

// WithWatchLogger sets the watcher's logger (default slog.Default).
func WithWatchLogger(l *slog.Logger) WatcherOption {
	return func(w *ConfigWatcher) { w.log = l }
}

// NewConfigWatcher builds a watcher for cfg. The config must be file-backed
// (loaded via [Load]); a bytes-parsed config never changes on disk.
func NewConfigWatcher(cfg *Config, opts ...WatcherOption) *ConfigWatcher {
	w := &ConfigWatcher{cfg: cfg, interval: 2 * time.Second, log: slog.Default()}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Name identifies the watcher as a component.
func (w *ConfigWatcher) Name() string { return "config-watcher" }

// Start records the current file stamp and begins polling.
func (w *ConfigWatcher) Start(context.Context) error {
	w.last = w.stamp()
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.wg.Add(1)
	go w.loop(ctx)
	return nil
}

// Stop ends the poll loop.
func (w *ConfigWatcher) Stop(context.Context) error {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	return nil
}

func (w *ConfigWatcher) loop(ctx context.Context) {
	defer w.wg.Done()
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.check(ctx)
		}
	}
}

func (w *ConfigWatcher) check(ctx context.Context) {
	s := w.stamp()
	if !s.ok || (s.mod.Equal(w.last.mod) && s.size == w.last.size) {
		return
	}
	w.last = s
	if err := w.cfg.Reload(); err != nil {
		w.log.LogAttrs(ctx, slog.LevelError, "config reload failed", slog.String("error", err.Error()))
		return
	}
	w.log.LogAttrs(ctx, slog.LevelInfo, "config reloaded", slog.String("path", w.cfg.path))
	if w.onReload != nil {
		w.onReload(w.cfg)
	}
}

func (w *ConfigWatcher) stamp() fileStamp {
	w.cfg.mu.RLock()
	path := w.cfg.path
	w.cfg.mu.RUnlock()
	if path == "" {
		return fileStamp{}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{mod: fi.ModTime(), size: fi.Size(), ok: true}
}
