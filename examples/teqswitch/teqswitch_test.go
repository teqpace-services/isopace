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

package main

import "testing"

// TestRunSwitch is a smoke test: client -> gateway -> host wires up, both
// transforms apply, and it shuts down cleanly.
func TestRunSwitch(t *testing.T) {
	if err := run(false, "127.0.0.1:0"); err != nil {
		t.Fatalf("run: %v", err)
	}
}
