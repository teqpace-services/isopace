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

package runtime

import (
	"context"
	"log/slog"
)

// Component is a managed unit with a start/stop lifecycle. Start should return
// promptly, launching any long-running work on its own goroutines; Stop must
// quiesce that work and release resources before it returns. Both calls are
// bounded by their ctx, so a slow component does not stall host shutdown
// indefinitely. Name identifies the component in logs and for undeploy.
//
// Implementations must tolerate Stop being called after a failed Start (so the
// host can unwind a partial startup) and need not be safe for concurrent
// Start/Stop — the host serialises lifecycle calls per component.
type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Env is the shared runtime context handed to component factories: the logger,
// the observability facade, and the root configuration. Passing it explicitly
// keeps components free of package-level globals and makes them testable in
// isolation. The zero Env is unusable; obtain one from a [Host] via [Host.Env].
type Env struct {
	Log    *slog.Logger
	Obs    Observer
	Config *Config
}

// Logger returns the environment logger, or the default logger if unset, so
// callers never have to nil-check.
func (e *Env) Logger() *slog.Logger {
	if e == nil || e.Log == nil {
		return slog.Default()
	}
	return e.Log
}

// Observer returns the environment observer, or a no-op observer if unset.
func (e *Env) Observer() Observer {
	if e == nil || e.Obs == nil {
		return NopObserver{}
	}
	return e.Obs
}
