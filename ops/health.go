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

package ops

import (
	"context"
	"sort"
	"sync"
)

// Status is the overall health verdict.
type Status string

const (
	// StatusHealthy means every registered check passed.
	StatusHealthy Status = "healthy"
	// StatusUnhealthy means at least one check failed.
	StatusUnhealthy Status = "unhealthy"
)

// CheckFunc is a single readiness probe; returning a non-nil error marks it
// failed. It is given a context so a probe can honour a deadline.
type CheckFunc func(ctx context.Context) error

// CheckResult is the outcome of one named check.
type CheckResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Report aggregates the results of all checks.
type Report struct {
	Status Status                 `json:"status"`
	Checks map[string]CheckResult `json:"checks"`
}

// Health is a registry of readiness checks. It is safe for concurrent use.
type Health struct {
	mu     sync.RWMutex
	checks map[string]CheckFunc
}

// NewHealth returns an empty health registry.
func NewHealth() *Health { return &Health{checks: map[string]CheckFunc{}} }

// Register adds (or replaces) a named check.
func (h *Health) Register(name string, fn CheckFunc) {
	h.mu.Lock()
	h.checks[name] = fn
	h.mu.Unlock()
}

// Deregister removes a check.
func (h *Health) Deregister(name string) {
	h.mu.Lock()
	delete(h.checks, name)
	h.mu.Unlock()
}

// Check runs every registered check and aggregates the results. With no checks
// registered the report is healthy.
func (h *Health) Check(ctx context.Context) Report {
	h.mu.RLock()
	snapshot := make(map[string]CheckFunc, len(h.checks))
	for name, fn := range h.checks {
		snapshot[name] = fn
	}
	h.mu.RUnlock()

	report := Report{Status: StatusHealthy, Checks: make(map[string]CheckResult, len(snapshot))}
	for name, fn := range snapshot {
		if err := fn(ctx); err != nil {
			report.Checks[name] = CheckResult{OK: false, Error: err.Error()}
			report.Status = StatusUnhealthy
		} else {
			report.Checks[name] = CheckResult{OK: true}
		}
	}
	return report
}

// Names returns the registered check names, sorted.
func (h *Health) Names() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.checks))
	for name := range h.checks {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
