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

package fieldcodec

import (
	"strconv"

	"github.com/teqpace-services/isopace/internal/bcd"
	"github.com/teqpace-services/isopace/internal/ebcdic"
	"github.com/teqpace-services/isopace/iso8583"
)

// Numeric value codecs. The canonical form is ASCII decimal digits; fixed fields
// are zero-padded on the left.

// numASCII is ASCII decimal digits (zero-copy decode).
type numASCII struct{}

func (numASCII) DecodeBody(body []byte, _ int, _ *iso8583.FieldDef) (iso8583.Value, error) {
	return iso8583.NumericValue(body), nil
}
func (numASCII) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	b, err := padFixed(v.Bytes(), def, '0', true)
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}
func (numASCII) Kind() iso8583.Kind { return iso8583.KindNumeric }
func (numASCII) Name() string       { return "num.ascii" }

// numEBCDIC is EBCDIC decimal digits (CP037; digits are page-invariant).
type numEBCDIC struct{}

func (numEBCDIC) DecodeBody(body []byte, _ int, _ *iso8583.FieldDef) (iso8583.Value, error) {
	return iso8583.NumericValue(ebcdic.CP037.Decode(body)), nil
}
func (numEBCDIC) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	b, err := padFixed(v.Bytes(), def, '0', true)
	if err != nil {
		return nil, err
	}
	return append(dst, ebcdic.CP037.Encode(b)...), nil
}
func (numEBCDIC) Kind() iso8583.Kind { return iso8583.KindNumeric }
func (numEBCDIC) Name() string       { return "num.ebcdic" }

// bcdNum is left-aligned packed BCD (two digits per byte). It implements
// WidthCodec so the engine derives the body byte span from the digit count.
type bcdNum struct{}

func (bcdNum) DecodeBody(body []byte, units int, def *iso8583.FieldDef) (iso8583.Value, error) {
	n := digitCount(units, def, body)
	digits, err := bcd.Unpack(body, n)
	if err != nil {
		return iso8583.Value{}, err
	}
	return iso8583.NumericValue(digits), nil
}
func (bcdNum) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	digits, err := padFixed(v.Bytes(), def, '0', true)
	if err != nil {
		return nil, err
	}
	packed, err := bcd.Pack(digits)
	if err != nil {
		return nil, ErrNonDigit
	}
	return append(dst, packed...), nil
}
func (bcdNum) Kind() iso8583.Kind      { return iso8583.KindNumeric }
func (bcdNum) Name() string            { return "num.bcd" }
func (bcdNum) BodyBytes(units int) int { return bcd.PackedLen(units) }

// rbcdNum is right-aligned packed BCD (odd digit padded in the trailing nibble).
type rbcdNum struct{}

func (rbcdNum) DecodeBody(body []byte, units int, def *iso8583.FieldDef) (iso8583.Value, error) {
	n := digitCount(units, def, body)
	digits, err := bcd.UnpackRight(body, n)
	if err != nil {
		return iso8583.Value{}, err
	}
	return iso8583.NumericValue(digits), nil
}
func (rbcdNum) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	digits, err := padFixed(v.Bytes(), def, '0', true)
	if err != nil {
		return nil, err
	}
	packed, err := bcd.PackRight(digits)
	if err != nil {
		return nil, ErrNonDigit
	}
	return append(dst, packed...), nil
}
func (rbcdNum) Kind() iso8583.Kind      { return iso8583.KindNumeric }
func (rbcdNum) Name() string            { return "num.rbcd" }
func (rbcdNum) BodyBytes(units int) int { return bcd.PackedLen(units) }

// binNum is a big-endian unsigned integer. The field's wire width is FieldDef
// .MaxLen BYTES (binary numerics are fixed-width).
type binNum struct{}

func (binNum) DecodeBody(body []byte, _ int, _ *iso8583.FieldDef) (iso8583.Value, error) {
	var n uint64
	for _, c := range body {
		n = n<<8 | uint64(c)
	}
	return iso8583.NumericValue([]byte(strconv.FormatUint(n, 10))), nil
}
func (binNum) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	n, err := v.Uint()
	if err != nil {
		return nil, err
	}
	width := def.MaxLen
	if width <= 0 {
		width = 8
	}
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = byte(n)
		n >>= 8
	}
	return append(dst, buf...), nil
}
func (binNum) Kind() iso8583.Kind { return iso8583.KindNumeric }
func (binNum) Name() string       { return "num.bin" }

// Catalog singletons.
var (
	NumASCII  iso8583.FieldCodec = numASCII{}
	NumEBCDIC iso8583.FieldCodec = numEBCDIC{}
	BCD       iso8583.FieldCodec = bcdNum{}
	RBCD      iso8583.FieldCodec = rbcdNum{}
	NumBinary iso8583.FieldCodec = binNum{}
)
