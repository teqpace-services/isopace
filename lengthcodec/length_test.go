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

package lengthcodec_test

import (
	"bytes"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/lengthcodec"
)

func TestKnownPrefixes(t *testing.T) {
	def := &iso8583.FieldDef{}
	cases := []struct {
		lc    iso8583.LengthCodec
		units int
		wire  []byte
	}{
		{lengthcodec.LLVarASCII, 19, []byte("19")},
		{lengthcodec.LLLVarASCII, 7, []byte("007")},
		{lengthcodec.LLLLVarASCII, 1234, []byte("1234")},
		{lengthcodec.LLVarBCD, 19, []byte{0x19}},
		{lengthcodec.LLLVarBCD, 123, []byte{0x01, 0x23}},
		{lengthcodec.LLVarBinary, 200, []byte{0xC8}},
		{lengthcodec.LLLVarBinary, 300, []byte{0x01, 0x2C}},
		{lengthcodec.LLVarEBCDIC, 19, []byte{0xF1, 0xF9}},
	}
	for _, c := range cases {
		got, err := c.lc.WriteLen(nil, c.units, def)
		if err != nil {
			t.Errorf("%s WriteLen(%d): %v", c.lc.Name(), c.units, err)
			continue
		}
		if !bytes.Equal(got, c.wire) {
			t.Errorf("%s WriteLen(%d) = % X want % X", c.lc.Name(), c.units, got, c.wire)
		}
		units, next, err := c.lc.ReadLen(c.wire, 0, def)
		if err != nil || units != c.units || next != len(c.wire) {
			t.Errorf("%s ReadLen(% X) = (%d,%d,%v) want (%d,%d,nil)",
				c.lc.Name(), c.wire, units, next, err, c.units, len(c.wire))
		}
	}
}

func TestFixed(t *testing.T) {
	def := &iso8583.FieldDef{MaxLen: 6}
	units, next, err := lengthcodec.Fixed.ReadLen([]byte("000123body"), 0, def)
	if err != nil || units != 6 || next != 0 {
		t.Errorf("Fixed.ReadLen = (%d,%d,%v) want (6,0,nil)", units, next, err)
	}
	out, err := lengthcodec.Fixed.WriteLen([]byte("x"), 6, def)
	if err != nil || string(out) != "x" {
		t.Errorf("Fixed.WriteLen wrote a prefix: %q", out)
	}
}

func TestRoundTripAll(t *testing.T) {
	def := &iso8583.FieldDef{}
	for _, lc := range lengthcodec.All() {
		if lc == lengthcodec.Fixed {
			continue
		}
		for _, u := range []int{0, 1, 9, 10, 99} {
			wire, err := lc.WriteLen(nil, u, def)
			if err != nil {
				t.Fatalf("%s WriteLen(%d): %v", lc.Name(), u, err)
			}
			got, next, err := lc.ReadLen(wire, 0, def)
			if err != nil || got != u || next != len(wire) {
				t.Errorf("%s round-trip %d -> % X -> (%d,%d,%v)", lc.Name(), u, wire, got, next, err)
			}
		}
	}
}

func TestOverflow(t *testing.T) {
	def := &iso8583.FieldDef{}
	if _, err := lengthcodec.LLVarASCII.WriteLen(nil, 100, def); err == nil {
		t.Errorf("LLVarASCII should overflow at 100")
	}
	if _, err := lengthcodec.LLVarBinary.WriteLen(nil, 256, def); err == nil {
		t.Errorf("LLVarBinary should overflow at 256")
	}
}
