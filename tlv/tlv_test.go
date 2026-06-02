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

package tlv_test

import (
	"testing"

	"github.com/teqpace-services/isopace/tlv"
)

// TestCoralPayDE62 exercises the exact CoralPay terminal-parameter shape: 2-char
// tag, 3-digit length. MID(15), currency(3), country(3), category(4), name(40).
func TestCoralPayDE62(t *testing.T) {
	elems := []tlv.Element{
		{Tag: "03", Value: "MERCHANT0000001"},                          // 15
		{Tag: "05", Value: "566"},                                      // 3
		{Tag: "06", Value: "566"},                                      // 3
		{Tag: "08", Value: "6011"},                                     // 4
		{Tag: "52", Value: "TEST MERCHANT, LAGOS, NG                "}, // 40
	}

	wire, err := tlv.Encode(elems, 2, 3)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if wire[:20] != "03015MERCHANT0000001" {
		t.Fatalf("encoded prefix = %q", wire[:20])
	}

	got, err := tlv.Decode(wire, 2, 3)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(got) != len(elems) {
		t.Fatalf("decoded %d elements, want %d", len(got), len(elems))
	}
	if mid, ok := tlv.Get(got, "03"); !ok || mid != "MERCHANT0000001" {
		t.Errorf("MID = %q, ok=%v", mid, ok)
	}
	if cur, _ := tlv.Get(got, "05"); cur != "566" {
		t.Errorf("currency = %q", cur)
	}
	if cat, _ := tlv.Get(got, "08"); cat != "6011" {
		t.Errorf("category = %q", cat)
	}
	if name, _ := tlv.Get(got, "52"); len(name) != 40 {
		t.Errorf("name length = %d, want 40", len(name))
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := tlv.Decode("0301", 2, 3); err == nil {
		t.Error("expected truncated-header error")
	}
	if _, err := tlv.Decode("03015AB", 2, 3); err == nil {
		t.Error("expected value-overrun error")
	}
	if _, err := tlv.Decode("03XYZvalue", 2, 3); err == nil {
		t.Error("expected bad-length error")
	}
	if _, err := tlv.Encode([]tlv.Element{{Tag: "3", Value: "x"}}, 2, 3); err == nil {
		t.Error("expected wrong-tag-width error")
	}
}
