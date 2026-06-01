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

package tlv

import "testing"

// TestBERLengthForms verifies appendBERLen and readTLV agree across the short
// form and all long-form widths — including >0xFFFFFF, which an earlier
// 3-byte-only encoder truncated even though readTLV accepted 4-byte lengths.
func TestBERLengthForms(t *testing.T) {
	for _, n := range []int{0, 1, 127, 128, 255, 256, 65535, 65536, 0xFFFFFF, 0x1000000, 0x7FABCDEF} {
		// A synthetic triplet: tag 0x95, length n, then n zero value bytes.
		buf := appendBERLen([]byte{0x95}, n)
		buf = append(buf, make([]byte, n)...)

		tag, val, next, err := readTLV(buf, 0)
		if err != nil {
			t.Fatalf("readTLV(len=%d): %v", n, err)
		}
		if tag != "95" {
			t.Errorf("len=%d tag = %q want 95", n, tag)
		}
		if len(val) != n {
			t.Errorf("len=%d decoded value length = %d (truncation?)", n, len(val))
		}
		if next != len(buf) {
			t.Errorf("len=%d next = %d want %d", n, next, len(buf))
		}
	}
}
