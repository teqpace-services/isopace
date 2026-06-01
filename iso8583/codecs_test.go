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

package iso8583

// Minimal in-package codecs that exercise the engine and the canonical value
// form. The shipped catalog (ASCII/BCD/EBCDIC/TLV, the full length-prefix and
// bitmap families) is Phase 2 in fieldcodec/ and lengthcodec/; these stubs only
// validate the Phase-1 core against the codec interfaces.

import (
	"encoding/binary"
	"fmt"
	"strconv"
)

// asciiStr — char.ascii (KindString). Fixed fields are space-padded on the right.
type asciiStr struct{}

func (asciiStr) DecodeBody(body []byte, _ *FieldDef) (Value, error) {
	return Value{raw: body, kind: KindString}, nil
}
func (asciiStr) EncodeBody(dst []byte, v Value, def *FieldDef) ([]byte, error) {
	b := v.raw
	if def.Length == nil {
		b = padFixed(b, def.MaxLen, ' ', false)
	}
	return append(dst, b...), nil
}
func (asciiStr) Kind() Kind   { return KindString }
func (asciiStr) Name() string { return "char.ascii" }

// asciiNum — num.ascii (KindNumeric). Fixed fields are zero-padded on the left.
type asciiNum struct{}

func (asciiNum) DecodeBody(body []byte, _ *FieldDef) (Value, error) {
	return Value{raw: body, kind: KindNumeric}, nil
}
func (asciiNum) EncodeBody(dst []byte, v Value, def *FieldDef) ([]byte, error) {
	b := v.raw
	if def.Length == nil {
		b = padFixed(b, def.MaxLen, '0', true)
	}
	return append(dst, b...), nil
}
func (asciiNum) Kind() Kind   { return KindNumeric }
func (asciiNum) Name() string { return "num.ascii" }

// asciiAmount — amount.ascii (KindAmount). Unsigned, zero-padded minor units.
type asciiAmount struct{ scale uint8 }

func (c asciiAmount) DecodeBody(body []byte, _ *FieldDef) (Value, error) {
	n, err := strconv.ParseInt(string(body), 10, 64)
	if err != nil {
		return Value{}, fmt.Errorf("amount: %w", err)
	}
	return Value{raw: body, kind: KindAmount, dec: NewDecimal(n, c.scale)}, nil
}
func (c asciiAmount) EncodeBody(dst []byte, v Value, def *FieldDef) ([]byte, error) {
	d, err := v.dec.Rescale(c.scale)
	if err != nil {
		return nil, err
	}
	digits := strconv.FormatInt(d.Unscaled, 10)
	return append(dst, padFixed([]byte(digits), def.MaxLen, '0', true)...), nil
}
func (c asciiAmount) Kind() Kind { return KindAmount }
func (asciiAmount) Name() string { return "amount.ascii" }

// binBytes — b.raw (KindBytes). Fixed fields are NUL-padded on the right.
type binBytes struct{}

func (binBytes) DecodeBody(body []byte, _ *FieldDef) (Value, error) {
	return Value{raw: body, kind: KindBytes}, nil
}
func (binBytes) EncodeBody(dst []byte, v Value, def *FieldDef) ([]byte, error) {
	b := v.raw
	if def.Length == nil {
		b = padFixed(b, def.MaxLen, 0x00, false)
	}
	return append(dst, b...), nil
}
func (binBytes) Kind() Kind   { return KindBytes }
func (binBytes) Name() string { return "b.raw" }

// padFixed pads b to width with pad on the left (left=true) or right; it leaves
// over-long input untouched (length validators catch overflow).
func padFixed(b []byte, width int, pad byte, left bool) []byte {
	if len(b) >= width {
		return b
	}
	out := make([]byte, width)
	gap := width - len(b)
	if left {
		for i := 0; i < gap; i++ {
			out[i] = pad
		}
		copy(out[gap:], b)
	} else {
		copy(out, b)
		for i := len(b); i < width; i++ {
			out[i] = pad
		}
	}
	return out
}

// llvarASCII — len.ll.ascii: a 2-digit ASCII length prefix (LLVAR). For these
// ASCII codecs one value unit equals one body byte.
type llvarASCII struct{}

func (llvarASCII) ReadLen(src []byte, off int, _ *FieldDef) (bodyLen, next int, err error) {
	if off+2 > len(src) {
		return 0, 0, errTruncated
	}
	n, err := strconv.Atoi(string(src[off : off+2]))
	if err != nil {
		return 0, 0, errBadLength
	}
	return n, off + 2, nil
}
func (llvarASCII) WriteLen(dst []byte, n int, _ *FieldDef) ([]byte, error) {
	if n < 0 || n > 99 {
		return nil, errBadLength
	}
	return append(dst, []byte(fmt.Sprintf("%02d", n))...), nil
}
func (llvarASCII) Name() string { return "len.ll.ascii" }

// binBitmap — bitmap.bin: 8/16/24-byte binary bitmap driven by the continuation
// bits (DE 1 -> secondary, DE 65 -> tertiary).
type binBitmap struct{}

const (
	maskDE1  = uint64(1) << 63 // continuation -> secondary present
	maskDE65 = uint64(1) << 63 // first bit of the secondary word -> tertiary present
)

func (binBitmap) ReadBitmap(src []byte, off, maxLevels int) (Bitmap, int, error) {
	var bm Bitmap
	if off+8 > len(src) {
		return bm, 0, errTruncated
	}
	w0 := binary.BigEndian.Uint64(src[off:])
	bm.SetWord(0, w0)
	next := off + 8
	if w0&maskDE1 != 0 && maxLevels >= 2 {
		if next+8 > len(src) {
			return bm, 0, errTruncated
		}
		w1 := binary.BigEndian.Uint64(src[next:])
		bm.SetWord(1, w1)
		next += 8
		if w1&maskDE65 != 0 && maxLevels >= 3 {
			if next+8 > len(src) {
				return bm, 0, errTruncated
			}
			bm.SetWord(2, binary.BigEndian.Uint64(src[next:]))
			next += 8
		}
	}
	return bm, next, nil
}

func (binBitmap) WriteBitmap(dst []byte, bm Bitmap, maxLevels int) ([]byte, error) {
	w0, w1, w2 := bm.Word(0), bm.Word(1), bm.Word(2)
	need3 := w2 != 0 && maxLevels >= 3
	need2 := (w1 != 0 || need3) && maxLevels >= 2
	if need2 {
		w0 |= maskDE1
	}
	if need3 {
		w1 |= maskDE65
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], w0)
	dst = append(dst, buf[:]...)
	if need2 {
		binary.BigEndian.PutUint64(buf[:], w1)
		dst = append(dst, buf[:]...)
		if need3 {
			binary.BigEndian.PutUint64(buf[:], w2)
			dst = append(dst, buf[:]...)
		}
	}
	return dst, nil
}
func (binBitmap) Name() string { return "bitmap.bin" }

// testSchema assembles an ASCII ISO-87-like profile from the stub codecs.
func testSchema() *Schema {
	return NewSchema("test-ascii").
		MTI(asciiNum{}).
		Bitmap(BitmapSpec{Codec: binBitmap{}, Levels: 2}).
		Field(2, "PAN", asciiNum{}, llvarASCII{}, MaxLen(19), Validate(Luhn())).
		Field(3, "Proc Code", asciiNum{}, nil, MaxLen(6)).
		Field(4, "Amount", asciiAmount{scale: 2}, nil, MaxLen(12), Validate(CurrencyAmount())).
		Field(11, "STAN", asciiNum{}, nil, MaxLen(6)).
		Field(35, "Track2", binBytes{}, llvarASCII{}, MaxLen(37)).
		Field(39, "Resp", asciiStr{}, nil, MaxLen(2)).
		Field(70, "Net Mgmt", asciiNum{}, nil, MaxLen(3)). // secondary-bitmap field
		MustBuild()
}
