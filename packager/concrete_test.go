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

package packager_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
)

// TestZoneRoundTrip exercises the "zone" packager end to end, including the DE
// 127 subfield group (binary sub-bitmap), a raw-binary field, and an ASCII-hex
// binary field.
func TestZoneRoundTrip(t *testing.T) {
	s := packager.Zone()
	c := iso8583.NewCodec(s)
	m := iso8583.New(s)
	pinBlock := []byte{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0} // DE 52 raw binary
	macSec := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04}   // DE 96 hex binary, full 8 octets
	tdsData := []byte{0xAA, 0xBB, 0xCC, 0xDD}                          // 127.29 hex binary: fixed 40 octets, NUL-padded
	wantTDS := make([]byte, 40)
	copy(wantTDS, tdsData)

	set(t, m, 0, "0200")
	set(t, m, 2, pan)
	set(t, m, 3, "000000")
	set(t, m, 4, int64(1099))
	set(t, m, 11, int64(123))
	set(t, m, 41, "TERM0001")
	set(t, m, 52, pinBlock)
	set(t, m, 96, macSec)
	setS(t, m, "127.2", "SWITCHKEY01")
	setS(t, m, "127.6", int64(12))
	setS(t, m, "127.8", "RETENTION-DATA")
	setS(t, m, "127.29", tdsData)

	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got, _ := iso8583.Get[string](m2, 2); got != pan {
		t.Errorf("DE2 PAN = %q want %q", got, pan)
	}
	if got, _ := iso8583.Get[int64](m2, 11); got != 123 {
		t.Errorf("DE11 STAN = %d want 123", got)
	}
	if got, _ := iso8583.Get[[]byte](m2, 52); !bytes.Equal(got, pinBlock) {
		t.Errorf("DE52 PIN = % X want % X", got, pinBlock)
	}
	if got, _ := iso8583.Get[[]byte](m2, 96); !bytes.Equal(got, macSec) {
		t.Errorf("DE96 (hex binary) = % X want % X", got, macSec)
	}
	if got, _ := iso8583.GetS[string](m2, "127.2"); got != "SWITCHKEY01" {
		t.Errorf("127.2 SWITCH KEY = %q", got)
	}
	if got, _ := iso8583.GetS[int64](m2, "127.6"); got != 12 {
		t.Errorf("127.6 AUTH PROFILE = %d want 12", got)
	}
	if got, _ := iso8583.GetS[string](m2, "127.8"); got != "RETENTION-DATA" {
		t.Errorf("127.8 RETENTION DATA = %q", got)
	}
	if got, _ := iso8583.GetS[[]byte](m2, "127.29"); !bytes.Equal(got, wantTDS) {
		t.Errorf("127.29 (hex binary) = % X want % X", got, wantTDS)
	}

	wire2, err := c.Marshal(m2, nil)
	if err != nil {
		t.Fatalf("re-Marshal: %v", err)
	}
	if !bytes.Equal(wire, wire2) {
		t.Errorf("zone wire not stable across round-trip")
	}
}

// TestSwitchRoundTrip exercises the "fields" / "switch" packagers (identical
// layouts) including the ASCII-hex bitmap and the DE 127 subfield group.
func TestSwitchRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		id string
		s  *iso8583.Schema
	}{
		{"fields", packager.Fields()},
		{"switch", packager.Switch()},
	} {
		t.Run(tc.id, func(t *testing.T) {
			c := iso8583.NewCodec(tc.s)
			m := iso8583.New(tc.s)
			pinBlock := []byte{0x0A, 0x1B, 0x2C, 0x3D, 0x4E, 0x5F, 0x60, 0x71} // DE 52 hex binary, full 8 octets
			tdsData := []byte{0x11, 0x22, 0x33, 0x44}                          // 127.29 hex binary: fixed 40 octets
			wantTDS := make([]byte, 40)
			copy(wantTDS, tdsData)

			set(t, m, 0, "0200")
			set(t, m, 2, pan)
			set(t, m, 3, "000000")
			set(t, m, 4, int64(2599))
			set(t, m, 11, int64(456))
			set(t, m, 41, "TERM0002")
			set(t, m, 52, pinBlock)
			setS(t, m, "127.7", "CHECK-DATA")
			setS(t, m, "127.6", int64(7))
			setS(t, m, "127.29", tdsData)

			wire, err := c.Marshal(m, nil)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			m2, err := c.Unmarshal(wire)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if got, _ := iso8583.Get[string](m2, 2); got != pan {
				t.Errorf("DE2 PAN = %q", got)
			}
			if got, _ := iso8583.Get[int64](m2, 11); got != 456 {
				t.Errorf("DE11 STAN = %d want 456", got)
			}
			if got, _ := iso8583.Get[[]byte](m2, 52); !bytes.Equal(got, pinBlock) {
				t.Errorf("DE52 (hex binary) = % X want % X", got, pinBlock)
			}
			if got, _ := iso8583.GetS[string](m2, "127.7"); got != "CHECK-DATA" {
				t.Errorf("127.7 CHECK DATA = %q", got)
			}
			if got, _ := iso8583.GetS[int64](m2, "127.6"); got != 7 {
				t.Errorf("127.6 AUTH PROFILE = %d want 7", got)
			}
			if got, _ := iso8583.GetS[[]byte](m2, "127.29"); !bytes.Equal(got, wantTDS) {
				t.Errorf("127.29 (hex binary) = % X want % X", got, wantTDS)
			}

			wire2, _ := c.Marshal(m2, nil)
			if !bytes.Equal(wire, wire2) {
				t.Errorf("%s wire not stable across round-trip", tc.id)
			}
		})
	}
}

// TestSubfieldReencode modifies a decoded DE 127 subfield and confirms the field
// re-encodes (the composite child re-marshals, other subfields preserved).
func TestSubfieldReencode(t *testing.T) {
	s := packager.Zone()
	c := iso8583.NewCodec(s)
	m := iso8583.New(s)
	set(t, m, 0, "0200")
	set(t, m, 2, pan)
	setS(t, m, "127.2", "ORIGKEY")
	setS(t, m, "127.6", int64(1))

	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Mutate one subfield on the decoded message; the rest must survive.
	setS(t, m2, "127.21", "REC-ID-123")

	wire2, err := c.Marshal(m2, nil)
	if err != nil {
		t.Fatalf("re-Marshal: %v", err)
	}
	m3, err := c.Unmarshal(wire2)
	if err != nil {
		t.Fatalf("re-Unmarshal: %v", err)
	}
	if got, _ := iso8583.GetS[string](m3, "127.2"); got != "ORIGKEY" {
		t.Errorf("127.2 lost after re-encode = %q", got)
	}
	if got, _ := iso8583.GetS[int64](m3, "127.6"); got != 1 {
		t.Errorf("127.6 lost after re-encode = %d", got)
	}
	if got, _ := iso8583.GetS[string](m3, "127.21"); got != "REC-ID-123" {
		t.Errorf("127.21 not added = %q", got)
	}
}

// TestImportedJSONMatchesGo asserts each generated schemadef JSON loads into a
// schema structurally identical to its programmatic profile, so the two forms
// stay in lockstep.
func TestImportedJSONMatchesGo(t *testing.T) {
	for _, tc := range []struct {
		file  string
		built *iso8583.Schema
	}{
		{"zone.json", packager.Zone()},
		{"fields.json", packager.Fields()},
		{"switch.json", packager.Switch()},
	} {
		t.Run(tc.file, func(t *testing.T) {
			js, err := packager.LoadEmbedded(tc.file)
			if err != nil {
				t.Fatalf("LoadEmbedded(%s): %v", tc.file, err)
			}
			assertSchemaEqual(t, tc.built.ID(), tc.built, js)

			// And the JSON-built schema round-trips a representative message.
			c := iso8583.NewCodec(js)
			m := iso8583.New(js)
			set(t, m, 0, "0200")
			set(t, m, 2, pan)
			setS(t, m, "127.6", int64(9))
			wire, err := c.Marshal(m, nil)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			m2, err := c.Unmarshal(wire)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got, _ := iso8583.GetS[int64](m2, "127.6"); got != 9 {
				t.Errorf("%s 127.6 = %d want 9", tc.file, got)
			}
		})
	}
}

func assertSchemaEqual(t *testing.T, name string, a, b *iso8583.Schema) {
	t.Helper()
	if a.MaxDE() != b.MaxDE() {
		t.Errorf("%s: MaxDE %d != %d", name, a.MaxDE(), b.MaxDE())
	}
	if a.BitmapSpec().Levels != b.BitmapSpec().Levels || bitmapName(a) != bitmapName(b) {
		t.Errorf("%s: bitmap %s/%d != %s/%d", name,
			bitmapName(a), a.BitmapSpec().Levels, bitmapName(b), b.BitmapSpec().Levels)
	}
	assertFieldDefEqual(t, name+" MTI", a.MTIDef(), b.MTIDef())

	hi := a.MaxDE()
	if b.MaxDE() > hi {
		hi = b.MaxDE()
	}
	for de := 1; de <= hi; de++ {
		ad, _ := a.Field(de)
		bd, _ := b.Field(de)
		assertFieldDefEqual(t, fmt.Sprintf("%s DE %d", name, de), ad, bd)
	}
}

func assertFieldDefEqual(t *testing.T, path string, a, b *iso8583.FieldDef) {
	t.Helper()
	if (a == nil) != (b == nil) {
		t.Errorf("%s: presence mismatch (go=%v json=%v)", path, a != nil, b != nil)
		return
	}
	if a == nil {
		return
	}
	if a.Name != b.Name {
		t.Errorf("%s: name %q != %q", path, a.Name, b.Name)
	}
	if a.Kind != b.Kind {
		t.Errorf("%s: kind %v != %v", path, a.Kind, b.Kind)
	}
	if a.MaxLen != b.MaxLen {
		t.Errorf("%s: maxlen %d != %d", path, a.MaxLen, b.MaxLen)
	}
	if codecName(a.Codec) != codecName(b.Codec) {
		t.Errorf("%s: codec %s != %s", path, codecName(a.Codec), codecName(b.Codec))
	}
	if lengthName(a.Length) != lengthName(b.Length) {
		t.Errorf("%s: length %s != %s", path, lengthName(a.Length), lengthName(b.Length))
	}
	if (a.Sub == nil) != (b.Sub == nil) {
		t.Errorf("%s: sub-schema presence mismatch", path)
		return
	}
	if a.Sub != nil {
		assertSchemaEqual(t, path+" sub", a.Sub, b.Sub)
	}
}

func bitmapName(s *iso8583.Schema) string {
	if c := s.BitmapSpec().Codec; c != nil {
		return c.Name()
	}
	return "<nil>"
}

func codecName(c iso8583.FieldCodec) string {
	if c == nil {
		return "<nil>"
	}
	return c.Name()
}

func lengthName(l iso8583.LengthCodec) string {
	if l == nil {
		return "len.fixed"
	}
	return l.Name()
}
