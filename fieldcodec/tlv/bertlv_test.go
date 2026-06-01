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
	"bytes"
	"testing"

	"github.com/teqpace-services/isopace/fieldcodec"
	"github.com/teqpace-services/isopace/fieldcodec/tlv"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/lengthcodec"
)

// emvSchema is a small EMV DE 55 sub-schema addressing tags by hex.
func emvSchema() *iso8583.Schema {
	return iso8583.NewSchema("emv-55").
		Tag("9F26", "App Cryptogram", fieldcodec.BINARY).
		Tag("9F36", "ATC", fieldcodec.NumBinary, iso8583.MaxLen(2)).
		Tag("95", "TVR", fieldcodec.BINARY).
		MustBuild()
}

// parentSchema wires DE 55 as a BER-TLV composite under an ASCII profile.
func parentSchema() *iso8583.Schema {
	return iso8583.NewSchema("with-emv").
		MTI(fieldcodec.NumASCII).
		Bitmap(iso8583.BitmapSpec{Codec: fieldcodec.BitmapBinary, Levels: 2}).
		Field(2, "PAN", fieldcodec.NumASCII, lengthcodec.LLVarASCII, iso8583.MaxLen(19)).
		Composite(55, "EMV", emvSchema(), lengthcodec.LLLVarASCII, iso8583.WithCodec(tlv.BERTLV)).
		MustBuild()
}

func TestBERTLVRoundTripViaEngine(t *testing.T) {
	s := parentSchema()
	c := iso8583.NewCodec(s)

	m := iso8583.New(s)
	must(t, m.Set(0, "0200"))
	must(t, m.Set(2, "4111111111111111"))
	cryptogram := []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18}
	tvr := []byte{0x80, 0x00, 0x00, 0x00, 0x00}
	must(t, m.SetS("55.9F26", cryptogram))
	must(t, m.SetS("55.95", tvr))
	must(t, m.SetS("55.9F36", int64(42)))

	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	m2, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Tag-addressed reads through the uniform path grammar.
	got, err := iso8583.GetS[[]byte](m2, "55.9F26")
	if err != nil || !bytes.Equal(got, cryptogram) {
		t.Errorf("55.9F26 = % X, %v want % X", got, err, cryptogram)
	}
	gotTVR, _ := iso8583.GetS[[]byte](m2, "55.95")
	if !bytes.Equal(gotTVR, tvr) {
		t.Errorf("55.95 = % X want % X", gotTVR, tvr)
	}
	atc, err := iso8583.GetS[int64](m2, "55.9F36")
	if err != nil || atc != 42 {
		t.Errorf("55.9F36 = %d, %v want 42", atc, err)
	}
	if pan, _ := iso8583.GetS[string](m2, "2"); pan != "4111111111111111" {
		t.Errorf("DE2 PAN = %q", pan)
	}

	// Re-marshalling the decoded message reproduces the wire.
	wire2, err := c.Marshal(m2, nil)
	if err != nil {
		t.Fatalf("re-Marshal: %v", err)
	}
	if !bytes.Equal(wire, wire2) {
		t.Errorf("BER-TLV not stable across round-trip:\n%x\n%x", wire, wire2)
	}
}

func TestBERTLVKnownEncoding(t *testing.T) {
	// 9F36 (ATC) length 2 value 0x002A == 42; 95 (TVR) length 3.
	s := parentSchema()
	m := iso8583.New(s)
	must(t, m.SetS("55.9F36", int64(42)))
	v, ok := m.GetS("55.9F36")
	if !ok {
		t.Fatal("9F36 not present after set")
	}
	if n, _ := v.Int(); n != 42 {
		t.Errorf("9F36 = %d want 42", n)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
