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

// Package store is the Isopace persistence layer: a small collection/key/value
// [Store] interface for the durable records an operational deployment keeps —
// RBAC policy, configuration snapshots, audit summaries — kept deliberately
// narrow so it maps onto many backends.
//
// [MemStore] is the in-process backend. A SQL-backed store is a drop-in adapter
// over the standard library's database/sql (the driver is the deployment's
// blank-import choice, so the core takes on no driver dependency); the durable
// space.Store is the queue analogue for store-and-forward. The [PutJSON] and
// [GetJSON] helpers marshal typed records over any Store.
package store
