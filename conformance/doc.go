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

// Package conformance holds the cross-package QA harness: golden ISO-8583 wire
// vectors, fuzz targets, and allocation/throughput benchmarks. The golden
// vectors are hand-derived from the ISO 8583 standard (clean-room — never
// captured from or copied out of any reference implementation; see
// CONTRIBUTING.md). It contains no shipped runtime code; everything lives in
// test files.
package conformance
