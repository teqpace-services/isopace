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
	"encoding/json"
	"net/http"

	"github.com/teqpace-services/isopace/rbac"
)

// Introspector exposes the running component names for the /info endpoint;
// *runtime.Host satisfies it.
type Introspector interface {
	Components() []string
}

// API is the HTTP admin/management surface. Build it with [NewAPI] and mount
// [API.Handler]. Liveness and metrics are unauthenticated; /info requires the
// "admin:read" permission when an rbac policy is configured (open otherwise, for
// local development).
type API struct {
	health       *Health
	metrics      *Metrics
	policy       *rbac.Policy
	introspector Introspector
	cluster      Cluster
	info         map[string]string
}

// APIOption configures an API.
type APIOption func(*API)

// WithHealth wires the readiness checks behind /readyz.
func WithHealth(h *Health) APIOption { return func(a *API) { a.health = h } }

// WithMetrics wires the metrics registry behind /metrics.
func WithMetrics(m *Metrics) APIOption { return func(a *API) { a.metrics = m } }

// WithAuth enables Basic-auth + permission checks on protected routes.
func WithAuth(p *rbac.Policy) APIOption { return func(a *API) { a.policy = p } }

// WithIntrospector adds the component list to /info.
func WithIntrospector(i Introspector) APIOption { return func(a *API) { a.introspector = i } }

// WithCluster adds membership to /info.
func WithCluster(c Cluster) APIOption { return func(a *API) { a.cluster = c } }

// WithInfo adds static key/value info (build version, etc.) to /info.
func WithInfo(info map[string]string) APIOption {
	return func(a *API) {
		a.info = map[string]string{}
		for k, v := range info {
			a.info[k] = v
		}
	}
}

// NewAPI builds an API.
func NewAPI(opts ...APIOption) *API {
	a := &API{}
	for _, o := range opts {
		o(a)
	}
	return a
}

// Handler returns the configured HTTP routes.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleLiveness)
	mux.HandleFunc("GET /readyz", a.handleReadiness)
	mux.HandleFunc("GET /metrics", a.handleMetrics)
	mux.HandleFunc("GET /info", a.protect("admin:read", a.handleInfo))
	return mux
}

func (a *API) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (a *API) handleReadiness(w http.ResponseWriter, r *http.Request) {
	report := Report{Status: StatusHealthy, Checks: map[string]CheckResult{}}
	if a.health != nil {
		report = a.health.Check(r.Context())
	}
	code := http.StatusOK
	if report.Status != StatusHealthy {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, report)
}

func (a *API) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if a.metrics != nil {
		_ = a.metrics.WriteText(w)
	}
}

func (a *API) handleInfo(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{}
	for k, v := range a.info {
		out[k] = v
	}
	if a.introspector != nil {
		out["components"] = a.introspector.Components()
	}
	if a.cluster != nil {
		out["cluster"] = a.cluster.Members()
	}
	writeJSON(w, http.StatusOK, out)
}

// protect wraps a handler with Basic-auth and a permission check when a policy
// is configured; without a policy the handler is served directly.
//
// Failures return 401 (unauthenticated) and authorization failures return 403
// (authenticated but lacking the permission) — the correct HTTP semantics.
// Because rbac.Authenticate runs in constant time and returns the same 401 for
// both an unknown user and a wrong password, username enumeration is not
// possible; a 403 only follows a valid credential. The remaining consideration
// is that a 403 confirms a supplied credential is valid, so a publicly exposed
// admin API should additionally be fronted by network controls (mTLS, an
// allow-list, or a private network) rather than relying on these checks alone.
func (a *API) protect(perm rbac.Permission, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.policy == nil {
			next(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || !a.policy.Authenticate(user, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="isopace"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !a.policy.Can(user, perm) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
