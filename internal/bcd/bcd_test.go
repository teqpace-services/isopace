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

package bcd

import (
	"bytes"
	"testing"
)

func TestPackKnownVectors(t *testing.T) {
	cases := []struct {
		in   string
		want []byte
	}{
		{"", []byte{}},
		{"0", []byte{0x00}},
		{"12345", []byte{0x01, 0x23, 0x45}}, // odd: left zero-pad
		{"1234", []byte{0x12, 0x34}},
		{"000000", []byte{0x00, 0x00, 0x00}},
		{"999999999999", []byte{0x99, 0x99, 0x99, 0x99, 0x99, 0x99}},
	}
	for _, c := range cases {
		got, err := Pack([]byte(c.in))
		if err != nil {
			t.Fatalf("Pack(%q): %v", c.in, err)
		}
		if !bytes.Equal(got, c.want) {
			t.Errorf("Pack(%q) = % x, want % x", c.in, got, c.want)
		}
	}
}

func TestPackUnpackRoundTrip(t *testing.T) {
	for _, s := range []string{"", "0", "5", "12345", "1234", "00", "0009", "999999999999"} {
		packed, err := Pack([]byte(s))
		if err != nil {
			t.Fatalf("Pack(%q): %v", s, err)
		}
		got, err := Unpack(packed, len(s))
		if err != nil {
			t.Fatalf("Unpack(% x, %d): %v", packed, len(s), err)
		}
		if string(got) != s {
			t.Errorf("round-trip %q -> % x -> %q", s, packed, got)
		}
	}
}

func TestPackNonDigit(t *testing.T) {
	for _, s := range []string{"12A4", "x", "1 2"} {
		if _, err := Pack([]byte(s)); err == nil {
			t.Errorf("Pack(%q) should reject non-digit", s)
		}
	}
}

func TestUnpackErrors(t *testing.T) {
	if _, err := Unpack([]byte{0xAB}, 2); err == nil {
		t.Error("Unpack(0xAB,2) should reject non-decimal nibble")
	}
	if _, err := Unpack([]byte{0x12}, 4); err == nil {
		t.Error("Unpack with too-short buffer should error")
	}
	if _, err := Unpack(nil, -1); err == nil {
		t.Error("Unpack with negative count should error")
	}
}
