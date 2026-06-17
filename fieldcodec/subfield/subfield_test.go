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

package subfield_test

import (
	"bytes"
	"testing"

	"github.com/teqpace-services/isopace/fieldcodec"
	"github.com/teqpace-services/isopace/fieldcodec/subfield"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/lengthcodec"
)

// parent wires DE 48 as a subfield packager over a headerless sub-schema with a
// binary sub-bitmap and three subfields of differing disciplines.
func parent(t *testing.T) *iso8583.Schema {
	t.Helper()
	sub := iso8583.NewSchema("sub").
		Headerless().
		Bitmap(iso8583.BitmapSpec{Codec: fieldcodec.BitmapBinary, Levels: 2}).
		Field(2, "Text", fieldcodec.ASCII, lengthcodec.LLVarASCII, iso8583.MaxLen(20)).
		Field(3, "Number", fieldcodec.NumASCII, nil, iso8583.MaxLen(4)).
		Field(4, "Octets", fieldcodec.BINARY, nil, iso8583.MaxLen(2)).
		MustBuild()
	return iso8583.NewSchema("p").
		MTI(fieldcodec.NumASCII).
		Bitmap(iso8583.BitmapSpec{Codec: fieldcodec.BitmapBinary, Levels: 2}).
		Composite(48, "Subfields", sub, lengthcodec.LLLVarASCII, iso8583.WithCodec(subfield.Packager)).
		MustBuild()
}

func TestSubfieldRoundTrip(t *testing.T) {
	s := parent(t)
	c := iso8583.NewCodec(s)
	m := iso8583.New(s)
	if err := m.Set(0, "0800"); err != nil {
		t.Fatalf("set MTI: %v", err)
	}
	mustSet(t, m, "48.2", "HELLO")
	mustSet(t, m, "48.3", int64(42))
	mustSet(t, m, "48.4", []byte{0xAB, 0xCD})

	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got, _ := iso8583.GetS[string](m2, "48.2"); got != "HELLO" {
		t.Errorf("48.2 = %q want HELLO", got)
	}
	if got, _ := iso8583.GetS[int64](m2, "48.3"); got != 42 {
		t.Errorf("48.3 = %d want 42", got)
	}
	if got, _ := iso8583.GetS[[]byte](m2, "48.4"); !bytes.Equal(got, []byte{0xAB, 0xCD}) {
		t.Errorf("48.4 = % X want AB CD", got)
	}

	wire2, err := c.Marshal(m2, nil)
	if err != nil {
		t.Fatalf("re-Marshal: %v", err)
	}
	if !bytes.Equal(wire, wire2) {
		t.Errorf("subfield wire not stable across round-trip")
	}
}

// TestSubfieldEncodeRejectsScalar guards the EncodeBody contract: a non-composite
// value cannot be serialised as a subfield packager.
func TestSubfieldEncodeRejectsScalar(t *testing.T) {
	def := &iso8583.FieldDef{Sub: iso8583.NewSchema("x").Headerless().
		Bitmap(iso8583.BitmapSpec{Codec: fieldcodec.BitmapBinary, Levels: 1}).
		Field(2, "F", fieldcodec.ASCII, nil, iso8583.MaxLen(1)).MustBuild()}
	if _, err := subfield.Packager.EncodeBody(nil, iso8583.BytesValue([]byte{0x01}), def); err == nil {
		t.Errorf("expected error encoding a non-composite value")
	}
}

func mustSet(t *testing.T, m *iso8583.Message, path string, v any) {
	t.Helper()
	if err := m.SetS(path, v); err != nil {
		t.Fatalf("SetS(%q): %v", path, err)
	}
}
