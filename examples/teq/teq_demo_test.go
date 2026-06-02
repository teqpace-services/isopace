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

package main

import "testing"

// TestRunDemo is a smoke test: the whole demo wires up, both switches connect,
// transactions round-trip, and it shuts down cleanly.
func TestRunDemo(t *testing.T) {
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
}
