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

// Package flow is the Isopace transaction manager: it runs a transaction
// through an ordered pipeline of [Stage] values in two phases — a prepare pass
// that may vote to continue, route, or abort, followed by a commit pass over the
// stages that prepared successfully (or an abort pass over them if any stage
// failed). Per-transaction state travels in an [Exchange].
//
// The two-phase model gives a transaction all-or-nothing semantics across
// participants: a stage does its reversible work and validation in Prepare, then
// makes effects durable in Commit (implementing [Committer]) or undoes reserved
// work in Abort (implementing [Aborter]). Stages route between named groups for
// conditional processing, and the [Flow] adds optional journaling, idempotent
// replay, per-stage retry, and a profiler — all built on the immutable
// copy-on-write iso8583.Message so a request is shared across stages without
// locking or defensive copies.
//
// An Exchange is processed by one goroutine at a time and is not safe for
// concurrent use; a Flow may run many exchanges concurrently.
package flow
