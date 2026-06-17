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

package ebcdic

import (
	"bytes"
	"testing"
)

func TestKnownVectors(t *testing.T) {
	cases := []struct {
		ascii  string
		ebcdic []byte
	}{
		{"0123456789", []byte{0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9}},
		{"ABC", []byte{0xC1, 0xC2, 0xC3}},
		{"XYZ", []byte{0xE7, 0xE8, 0xE9}},
		{"abc", []byte{0x81, 0x82, 0x83}},
		{"jr", []byte{0x91, 0x99}},
		{" ", []byte{0x40}},
		{"4111", []byte{0xF4, 0xF1, 0xF1, 0xF1}},
	}
	for _, c := range cases {
		if got := CP037.Encode([]byte(c.ascii)); !bytes.Equal(got, c.ebcdic) {
			t.Errorf("CP037.Encode(%q) = % X want % X", c.ascii, got, c.ebcdic)
		}
		if got := CP037.Decode(c.ebcdic); string(got) != c.ascii {
			t.Errorf("CP037.Decode(% X) = %q want %q", c.ebcdic, got, c.ascii)
		}
	}
}

func TestRoundTripInvariantSubset(t *testing.T) {
	// Every ASCII byte that has an assigned EBCDIC code must round-trip on both
	// code pages.
	for _, tbl := range []*Table{CP037, CP1047} {
		for a := 0; a < 256; a++ {
			e := tbl.a2e[a]
			if e == ebcdicQ && byte(a) != '?' {
				continue // unassigned ASCII byte
			}
			if got := tbl.e2a[e]; got != byte(a) {
				t.Errorf("round-trip failed: ascii %#x -> ebcdic %#x -> ascii %#x", a, e, got)
			}
		}
	}
}

func TestCodePageDifferences(t *testing.T) {
	// '^' is representable in CP1047 but not CP037.
	if CP1047.EncodeByte('^') != 0x5F {
		t.Errorf("CP1047 '^' = %#x want 0x5F", CP1047.EncodeByte('^'))
	}
	// Brackets sit at different code points per page.
	if CP1047.DecodeByte(0xAD) != '[' || CP1047.DecodeByte(0xBD) != ']' {
		t.Errorf("CP1047 bracket mapping wrong")
	}
	if CP037.DecodeByte(0xBA) != '[' || CP037.DecodeByte(0xBB) != ']' {
		t.Errorf("CP037 bracket mapping wrong")
	}
}

func TestUnmappableFallback(t *testing.T) {
	// An EBCDIC byte with no assignment decodes to '?'.
	if CP037.DecodeByte(0x05) != '?' {
		t.Errorf("unassigned EBCDIC byte did not decode to '?'")
	}
}
