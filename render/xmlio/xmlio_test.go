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

package xmlio_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/render/xmlio"
)

const pan = "4111111111111111"

func sample(t *testing.T, s *iso8583.Schema) *iso8583.Message {
	t.Helper()
	m := iso8583.New(s)
	for _, kv := range []struct {
		de int
		v  any
	}{
		{0, "0200"}, {2, pan}, {3, "000000"}, {4, iso8583.NewDecimal(1099, 0)},
		{11, int64(123)}, {41, "TERM0001"},
	} {
		if err := m.Set(kv.de, kv.v); err != nil {
			t.Fatalf("Set(%d): %v", kv.de, err)
		}
	}
	if err := m.SetS("55.9F26", []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18}); err != nil {
		t.Fatalf("SetS 9F26: %v", err)
	}
	if err := m.SetS("55.9F36", int64(7)); err != nil {
		t.Fatalf("SetS 9F36: %v", err)
	}
	return m
}

func TestXMLRoundTrip(t *testing.T) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	m := sample(t, s)

	data, err := xmlio.Marshal(m, xmlio.UseNames())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, `<field id="0" value="0200"`) {
		t.Errorf("XML missing MTI field:\n%s", out)
	}
	if !strings.Contains(out, `<isomsg id="55"`) {
		t.Errorf("XML missing nested DE 55 composite:\n%s", out)
	}
	if !strings.Contains(out, `id="9F26" value="A1B2C3D4E5F60718" type="binary"`) {
		t.Errorf("binary TLV tag not rendered as hex:\n%s", out)
	}
	if !strings.Contains(out, `name="Primary Account Number"`) {
		t.Errorf("UseNames did not emit field names:\n%s", out)
	}

	m2, err := xmlio.Unmarshal(data, s)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// The XML round-trip must reconstruct a wire-identical message.
	wireA, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal m: %v", err)
	}
	wireB, err := c.Marshal(m2, nil)
	if err != nil {
		t.Fatalf("Marshal m2: %v", err)
	}
	if !bytes.Equal(wireA, wireB) {
		t.Errorf("XML round-trip not wire-identical:\n A=%x\n B=%x", wireA, wireB)
	}
}

// TestXMLAmountScale checks that a fractional amount round-trips its exact scale
// through the flat string value attribute.
func TestXMLAmountScale(t *testing.T) {
	s := packager.ISO87A()
	m := iso8583.New(s)
	if err := m.Set(0, "0200"); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(4, iso8583.NewDecimal(1099, 2)); err != nil { // 10.99
		t.Fatal(err)
	}
	data, err := xmlio.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `id="4" value="10.99"`) {
		t.Errorf("amount not rendered as decimal string:\n%s", data)
	}
	m2, err := xmlio.Unmarshal(data, s)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	v, _ := m2.Get(4)
	d, err := v.Decimal()
	if err != nil {
		t.Fatalf("Decimal: %v", err)
	}
	if d.Unscaled != 1099 || d.Scale != 2 {
		t.Errorf("amount scale lost: got {%d,%d}, want {1099,2}", d.Unscaled, d.Scale)
	}
}

func TestXMLMaskPAN(t *testing.T) {
	s := packager.ISO87A()
	m := sample(t, s)
	data, err := xmlio.Marshal(m, xmlio.MaskPAN())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), "411111******1111") {
		t.Errorf("PAN not masked:\n%s", data)
	}
	if strings.Contains(string(data), pan) {
		t.Errorf("unmasked PAN leaked into output")
	}
}

// TestXMLDeterministic guards the TLV round-trip: the reconstructed wire must be
// identical across many parses (document order, not random map iteration).
func TestXMLDeterministic(t *testing.T) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	data, err := xmlio.Marshal(sample(t, s))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var first []byte
	for i := 0; i < 64; i++ {
		m, err := xmlio.Unmarshal(data, s)
		if err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		wire, err := c.Marshal(m, nil)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if first == nil {
			first = wire
		} else if !bytes.Equal(first, wire) {
			t.Fatalf("non-deterministic round-trip at iteration %d:\n first=%x\n  got=%x", i, first, wire)
		}
	}
}

// TestXMLEscaping checks that XML metacharacters in a value survive the round
// trip rather than corrupting the document.
func TestXMLEscaping(t *testing.T) {
	s := packager.ISO87A()
	m := iso8583.New(s)
	if err := m.Set(0, "0200"); err != nil {
		t.Fatal(err)
	}
	const tricky = `a<b>&"c'd`
	if err := m.Set(43, tricky); err != nil { // DE 43 is alphanumeric text
		t.Fatal(err)
	}
	data, err := xmlio.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "a<b>") {
		t.Errorf("metacharacters not escaped:\n%s", data)
	}
	m2, err := xmlio.Unmarshal(data, s)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	v, _ := m2.Get(43)
	got, _ := v.String()
	if strings.TrimRight(got, " ") != tricky {
		t.Errorf("escaped value not recovered: got %q want %q", got, tricky)
	}
}

func TestXMLRejectsNonIsomsgRoot(t *testing.T) {
	s := packager.ISO87A()
	if _, err := xmlio.Unmarshal([]byte(`<message><field id="0" value="0200"/></message>`), s); err == nil {
		t.Errorf("expected error for non-isomsg root, got nil")
	}
}
