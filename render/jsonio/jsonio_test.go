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

package jsonio_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/render/jsonio"
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

func TestJSONRoundTrip(t *testing.T) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	m := sample(t, s)

	data, err := jsonio.Marshal(m, jsonio.UseNames())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"mti": "0200"`) {
		t.Errorf("JSON missing MTI:\n%s", data)
	}
	if !strings.Contains(string(data), `"Primary Account Number"`) {
		t.Errorf("UseNames did not emit field names")
	}

	m2, err := jsonio.Unmarshal(data, s)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// The JSON round-trip must reconstruct a wire-identical message.
	wireA, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal m: %v", err)
	}
	wireB, err := c.Marshal(m2, nil)
	if err != nil {
		t.Fatalf("Marshal m2: %v", err)
	}
	if !bytes.Equal(wireA, wireB) {
		t.Errorf("JSON round-trip not wire-identical:\n A=%x\n B=%x", wireA, wireB)
	}
}

func TestJSONMaskPAN(t *testing.T) {
	s := packager.ISO87A()
	m := sample(t, s)
	data, err := jsonio.Marshal(m, jsonio.MaskPAN())
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

// TestJSONUnmarshalDeterministic guards against random map iteration of TLV
// tags making the reconstructed wire non-deterministic. The TLV object is
// re-parsed many times; every reconstruction must yield identical wire.
func TestJSONUnmarshalDeterministic(t *testing.T) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	data, err := jsonio.Marshal(sample(t, s))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var first []byte
	for i := 0; i < 64; i++ {
		m, err := jsonio.Unmarshal(data, s)
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
			t.Fatalf("non-deterministic TLV round-trip at iteration %d:\n first=%x\n  got=%x", i, first, wire)
		}
	}
}

// TestJSONTLVOnNonComposite ensures malformed JSON (tlv on a scalar DE) errors
// cleanly instead of dereferencing a nil sub-schema.
func TestJSONTLVOnNonComposite(t *testing.T) {
	s := packager.ISO87A()
	bad := []byte(`{"mti":"0200","fields":{"4":{"tlv":{"9F26":"A1B2"}}}}`)
	if _, err := jsonio.Unmarshal(bad, s); err == nil {
		t.Errorf("expected error for tlv on non-composite DE 4, got nil (panic risk)")
	}
}

func TestJSONAmountShape(t *testing.T) {
	s := packager.ISO87A()
	m := sample(t, s)
	data, _ := jsonio.Marshal(m)
	if !strings.Contains(string(data), `"unscaled": 1099`) {
		t.Errorf("amount not rendered as scaled object:\n%s", data)
	}
}
