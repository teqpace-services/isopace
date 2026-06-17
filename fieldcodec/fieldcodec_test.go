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

package fieldcodec_test

import (
	"bytes"
	"testing"

	"github.com/teqpace-services/isopace/fieldcodec"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/lengthcodec"
)

// fixed returns a fixed-width FieldDef (no length codec) of the given width.
func fixed(maxLen int) *iso8583.FieldDef { return &iso8583.FieldDef{MaxLen: maxLen} }

// variable returns a FieldDef with a (non-nil) length codec so codecs skip
// fixed-field padding.
func variable() *iso8583.FieldDef { return &iso8583.FieldDef{Length: lengthcodec.LLVarASCII} }

func TestCharCodecs(t *testing.T) {
	// ASCII fixed: right space-padded.
	out, err := fieldcodec.ASCII.EncodeBody(nil, iso8583.StringValue([]byte("AB")), fixed(5))
	if err != nil || string(out) != "AB   " {
		t.Errorf("ASCII fixed encode = %q, %v want \"AB   \"", out, err)
	}
	v, _ := fieldcodec.ASCII.DecodeBody([]byte("AB   "), 5, fixed(5))
	if s, _ := v.String(); s != "AB   " {
		t.Errorf("ASCII decode = %q", s)
	}

	// EBCDIC037 round-trip "ABC".
	eb, err := fieldcodec.EBCDIC037.EncodeBody(nil, iso8583.StringValue([]byte("ABC")), fixed(3))
	if err != nil || !bytes.Equal(eb, []byte{0xC1, 0xC2, 0xC3}) {
		t.Errorf("EBCDIC037 encode = % X, %v want C1 C2 C3", eb, err)
	}
	dv, _ := fieldcodec.EBCDIC037.DecodeBody(eb, 3, fixed(3))
	if s, _ := dv.String(); s != "ABC" {
		t.Errorf("EBCDIC037 decode = %q", s)
	}

	// BINARY is zero-copy bytes.
	bv, _ := fieldcodec.BINARY.DecodeBody([]byte{0x01, 0x02}, 2, variable())
	if !bytes.Equal(bv.Bytes(), []byte{0x01, 0x02}) {
		t.Errorf("BINARY decode = % X", bv.Bytes())
	}
}

func TestHexBinaryCodec(t *testing.T) {
	// b.hex: octets out as uppercase ASCII hex, NUL-padded to the fixed width.
	out, err := fieldcodec.HexBINARY.EncodeBody(nil, iso8583.BytesValue([]byte{0xDE, 0xAD}), fixed(4))
	if err != nil || string(out) != "DEAD0000" {
		t.Errorf("HexBINARY encode = %q, %v want DEAD0000", out, err)
	}
	// Decode parses 2N hex chars back to N octets.
	v, err := fieldcodec.HexBINARY.DecodeBody([]byte("DEAD0000"), 4, fixed(4))
	if err != nil || !bytes.Equal(v.Bytes(), []byte{0xDE, 0xAD, 0x00, 0x00}) {
		t.Errorf("HexBINARY decode = % X, %v", v.Bytes(), err)
	}
	// Wire span is two hex chars per logical octet.
	if wc, ok := fieldcodec.HexBINARY.(iso8583.WidthCodec); !ok || wc.BodyBytes(8) != 16 {
		t.Errorf("HexBINARY BodyBytes(8) != 16")
	}
	// Malformed hex is rejected.
	if _, err := fieldcodec.HexBINARY.DecodeBody([]byte("ZZ"), 1, fixed(1)); err == nil {
		t.Errorf("expected error on non-hex body")
	}
}

func TestNumericCodecs(t *testing.T) {
	// num.ascii fixed: zero left-padded.
	out, _ := fieldcodec.NumASCII.EncodeBody(nil, iso8583.NumericValue([]byte("123")), fixed(6))
	if string(out) != "000123" {
		t.Errorf("NumASCII fixed = %q want 000123", out)
	}

	// BCD even and odd, fixed.
	bcdEven, _ := fieldcodec.BCD.EncodeBody(nil, iso8583.NumericValue([]byte("123456")), fixed(6))
	if !bytes.Equal(bcdEven, []byte{0x12, 0x34, 0x56}) {
		t.Errorf("BCD even = % X want 12 34 56", bcdEven)
	}
	bcdOdd, _ := fieldcodec.BCD.EncodeBody(nil, iso8583.NumericValue([]byte("12345")), fixed(5))
	if !bytes.Equal(bcdOdd, []byte{0x01, 0x23, 0x45}) {
		t.Errorf("BCD odd = % X want 01 23 45 (left pad)", bcdOdd)
	}
	dv, _ := fieldcodec.BCD.DecodeBody(bcdOdd, 5, fixed(5))
	if s, _ := dv.String(); s != "12345" {
		t.Errorf("BCD odd decode = %q want 12345", s)
	}

	// RBCD odd pads the trailing nibble.
	rbcdOdd, _ := fieldcodec.RBCD.EncodeBody(nil, iso8583.NumericValue([]byte("12345")), fixed(5))
	if !bytes.Equal(rbcdOdd, []byte{0x12, 0x34, 0x50}) {
		t.Errorf("RBCD odd = % X want 12 34 50 (right pad)", rbcdOdd)
	}
	rv, _ := fieldcodec.RBCD.DecodeBody(rbcdOdd, 5, fixed(5))
	if s, _ := rv.String(); s != "12345" {
		t.Errorf("RBCD odd decode = %q want 12345", s)
	}

	// num.bin big-endian.
	binOut, _ := fieldcodec.NumBinary.EncodeBody(nil, iso8583.NumericValue([]byte("258")), fixed(2))
	if !bytes.Equal(binOut, []byte{0x01, 0x02}) {
		t.Errorf("NumBinary = % X want 01 02", binOut)
	}
	nv, _ := fieldcodec.NumBinary.DecodeBody([]byte{0x01, 0x02}, 2, fixed(2))
	if n, _ := nv.Int(); n != 258 {
		t.Errorf("NumBinary decode = %d want 258", n)
	}
}

func TestWidthCodec(t *testing.T) {
	wc, ok := fieldcodec.BCD.(iso8583.WidthCodec)
	if !ok {
		t.Fatal("BCD must implement WidthCodec")
	}
	if wc.BodyBytes(5) != 3 || wc.BodyBytes(6) != 3 || wc.BodyBytes(12) != 6 {
		t.Errorf("BCD BodyBytes wrong: 5->%d 6->%d 12->%d", wc.BodyBytes(5), wc.BodyBytes(6), wc.BodyBytes(12))
	}
}

func TestAmountCodecs(t *testing.T) {
	def := &iso8583.FieldDef{MaxLen: 12, Scale: 2}
	for name, c := range map[string]iso8583.FieldCodec{
		"ascii": fieldcodec.AmountASCII,
		"bcd":   fieldcodec.AmountBCD,
	} {
		wire, err := c.EncodeBody(nil, iso8583.AmountValue(iso8583.NewDecimal(1099, 2), nil), def)
		if err != nil {
			t.Fatalf("%s encode: %v", name, err)
		}
		units := 12
		dv, err := c.DecodeBody(wire, units, def)
		if err != nil {
			t.Fatalf("%s decode: %v", name, err)
		}
		d, _ := dv.Decimal()
		if d.String() != "10.99" {
			t.Errorf("%s amount round-trip = %s want 10.99", name, d.String())
		}
	}
	// ASCII amount wire is zero-padded digits.
	wire, _ := fieldcodec.AmountASCII.EncodeBody(nil, iso8583.AmountValue(iso8583.NewDecimal(1099, 2), nil), def)
	if string(wire) != "000000001099" {
		t.Errorf("AmountASCII wire = %q want 000000001099", wire)
	}
	// Negative amounts are rejected (unsigned field).
	if _, err := fieldcodec.AmountASCII.EncodeBody(nil, iso8583.AmountValue(iso8583.NewDecimal(-5, 2), nil), def); err == nil {
		t.Errorf("negative amount should be rejected")
	}
}

func TestBitmapCodecs(t *testing.T) {
	var bm iso8583.Bitmap
	bm.Set(2)
	bm.Set(3)
	bm.Set(70) // secondary

	for _, bc := range []iso8583.BitmapCodec{fieldcodec.BitmapBinary, fieldcodec.BitmapHex, fieldcodec.BitmapEBCDIC} {
		wire, err := bc.WriteBitmap(nil, bm, 2)
		if err != nil {
			t.Fatalf("%s write: %v", bc.Name(), err)
		}
		got, next, err := bc.ReadBitmap(wire, 0, 2)
		if err != nil || next != len(wire) {
			t.Fatalf("%s read: next=%d len=%d err=%v", bc.Name(), next, len(wire), err)
		}
		if !got.IsSet(2) || !got.IsSet(3) || !got.IsSet(70) || got.Count() != 3 {
			t.Errorf("%s round-trip lost fields: count=%d", bc.Name(), got.Count())
		}
	}

	// Primary-only stays one level.
	var p iso8583.Bitmap
	p.Set(2)
	wire, _ := fieldcodec.BitmapBinary.WriteBitmap(nil, p, 2)
	if len(wire) != 8 {
		t.Errorf("primary-only binary bitmap = %d bytes want 8", len(wire))
	}
	wireHex, _ := fieldcodec.BitmapHex.WriteBitmap(nil, p, 2)
	if len(wireHex) != 16 {
		t.Errorf("primary-only hex bitmap = %d chars want 16", len(wireHex))
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := fieldcodec.DefaultRegistry()
	for _, n := range []string{"char.ascii", "char.ebcdic.cp037", "num.bcd", "num.rbcd", "amount.bcd", "b.raw"} {
		if _, ok := r.Lookup(n); !ok {
			t.Errorf("registry missing value codec %q", n)
		}
	}
	for _, n := range []string{"len.fixed", "len.ll.ascii", "len.lll.bcd", "len.ll.bin", "len.ll.ebcdic"} {
		if _, ok := r.LookupLength(n); !ok {
			t.Errorf("registry missing length codec %q", n)
		}
	}
	for _, n := range []string{"bitmap.bin", "bitmap.hex", "bitmap.ebcdic"} {
		if _, ok := r.LookupBitmap(n); !ok {
			t.Errorf("registry missing bitmap codec %q", n)
		}
	}
}
