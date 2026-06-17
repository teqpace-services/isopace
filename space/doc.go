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

// Package space is the Isopace coordination layer: a keyed tuple space for
// decoupling producers and consumers (queues, work hand-off, request/response
// rendezvous) across the components of a deployment.
//
// The [Space] interface follows the classic associative-memory primitives:
// Out writes an entry under a key, In takes (removes and returns) one entry and
// blocks until one is available, Rd reads (copies) one without removing it, and
// Inp/Rdp are their non-blocking probes. A key holds a FIFO bag, so a key is a
// queue and Out/In is producer/consumer hand-off. Blocking calls take a context,
// so waits are cancelable and bounded.
//
// # Backends
//
//   - [Local] is the in-process backend (a map guarded by a mutex, with a
//     broadcast that wakes blocked waiters). It implements [Space].
//   - [Store] is durable store-and-forward: a []byte queue backed by a crash-safe
//     append-only log that replays on open, for messages that must survive a
//     restart (e.g. queued for an endpoint that is currently down). Its API
//     returns errors because disk I/O can fail, so it is a sibling of Space
//     rather than an implementation of it.
//
// A distributed backend — NATS / JetStream — is a drop-in adapter that
// implements [Space] without the core module taking on the dependency, keeping
// Isopace stdlib-only. It would live in a separate, optionally-imported module.
package space
