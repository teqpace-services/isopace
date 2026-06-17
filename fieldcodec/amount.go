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
	"github.com/teqpace-services/isopace/iso8583"
)

// Amount value codecs. An amount field decodes to an exact Decimal whose scale
// comes from FieldDef.Scale (minor-unit digits). ISO-8583 transaction amounts
// are unsigned on the wire; a negative value is rejected on encode.

// amountDigits renders an unsigned amount's unscaled minor units as ASCII.
func amountDigits(v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	d, err := v.Decimal()
	if err != nil {
		return nil, err
	}
	d, err = d.Rescale(def.Scale)
	if err != nil {
		return nil, err
	}
	if d.Unscaled < 0 {
		return nil, ErrNegative
	}
	return []byte(strconv.FormatInt(d.Unscaled, 10)), nil
}

// amountASCII is an ASCII-digit amount, zero-padded to MaxLen.
type amountASCII struct{}

func (amountASCII) DecodeBody(body []byte, _ int, def *iso8583.FieldDef) (iso8583.Value, error) {
	n, err := strconv.ParseInt(string(body), 10, 64)
	if err != nil {
		return iso8583.Value{}, ErrNonDigit
	}
	return iso8583.AmountValue(iso8583.NewDecimal(n, def.Scale), body), nil
}
func (amountASCII) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	digits, err := amountDigits(v, def)
	if err != nil {
		return nil, err
	}
	b, err := padFixed(digits, def, '0', true)
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}
func (amountASCII) Kind() iso8583.Kind { return iso8583.KindAmount }
func (amountASCII) Name() string       { return "amount.ascii" }

// amountBCD is a packed-BCD amount.
type amountBCD struct{}

func (amountBCD) DecodeBody(body []byte, units int, def *iso8583.FieldDef) (iso8583.Value, error) {
	n := digitCount(units, def, body)
	digits, err := bcd.Unpack(body, n)
	if err != nil {
		return iso8583.Value{}, err
	}
	val, err := strconv.ParseInt(string(digits), 10, 64)
	if err != nil {
		return iso8583.Value{}, ErrNonDigit
	}
	return iso8583.AmountValue(iso8583.NewDecimal(val, def.Scale), digits), nil
}
func (amountBCD) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	digits, err := amountDigits(v, def)
	if err != nil {
		return nil, err
	}
	if digits, err = padFixed(digits, def, '0', true); err != nil {
		return nil, err
	}
	packed, err := bcd.Pack(digits)
	if err != nil {
		return nil, ErrNonDigit
	}
	return append(dst, packed...), nil
}
func (amountBCD) Kind() iso8583.Kind      { return iso8583.KindAmount }
func (amountBCD) Name() string            { return "amount.bcd" }
func (amountBCD) BodyBytes(units int) int { return bcd.PackedLen(units) }

// amountBinary is a big-endian unsigned amount in MaxLen bytes.
type amountBinary struct{}

func (amountBinary) DecodeBody(body []byte, _ int, def *iso8583.FieldDef) (iso8583.Value, error) {
	var n uint64
	for _, c := range body {
		n = n<<8 | uint64(c)
	}
	return iso8583.AmountValue(iso8583.NewDecimal(int64(n), def.Scale), body), nil
}
func (amountBinary) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	d, err := v.Decimal()
	if err != nil {
		return nil, err
	}
	if d, err = d.Rescale(def.Scale); err != nil {
		return nil, err
	}
	if d.Unscaled < 0 {
		return nil, ErrNegative
	}
	width := def.MaxLen
	if width <= 0 {
		width = 8
	}
	n := uint64(d.Unscaled)
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = byte(n)
		n >>= 8
	}
	return append(dst, buf...), nil
}
func (amountBinary) Kind() iso8583.Kind { return iso8583.KindAmount }
func (amountBinary) Name() string       { return "amount.bin" }

// Catalog singletons.
var (
	AmountASCII  iso8583.FieldCodec = amountASCII{}
	AmountBCD    iso8583.FieldCodec = amountBCD{}
	AmountBinary iso8583.FieldCodec = amountBinary{}
)
