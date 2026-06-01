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

// Package runtime is the Isopace component host: it owns the lifecycle of the
// long-running pieces of a deployment (links, listeners, switches, flows) and
// the cross-cutting plumbing they share — configuration, structured logging
// (log/slog), and an observability facade for traces and metrics.
//
// A [Host] starts registered [Component] values in registration order and stops
// them in reverse, so dependencies come up before dependents and go down after.
// Components can also be deployed and undeployed while the host runs, which the
// [Deployer] uses to turn a directory of declarative JSON [Descriptor] files
// into running components and to hot-redeploy them when the files change.
//
// # Dependency policy
//
// The package is stdlib-only, like the rest of Isopace. Observability is exposed
// as the [Observer] interface with a no-op default and an slog-backed
// implementation; a real OpenTelemetry exporter is a drop-in adapter that
// implements [Observer] without the core module taking on the dependency.
// Configuration loads from JSON plus environment overrides; YAML is a drop-in
// once a permissively licensed parser is vendored.
package runtime
