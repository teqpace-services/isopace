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

// Package ops is the Isopace operational surface: liveness/readiness health
// checks ([Health]), a metrics registry with Prometheus text exposition
// ([Metrics], which also implements the runtime.Observer facade), cluster
// membership ([Cluster]), and an HTTP admin/management API ([API]) that ties
// them together and is guarded by rbac when a policy is supplied.
//
// Everything is stdlib-only (net/http, encoding/json). A distributed cluster
// membership backend (gossip/raft, or one built on the space NATS adapter) is a
// drop-in [Cluster] implementation; [StaticCluster] is the single-node / static
// default.
package ops
