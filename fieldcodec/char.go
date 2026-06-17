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
	"github.com/teqpace-services/isopace/internal/ebcdic"
	"github.com/teqpace-services/isopace/iso8583"
)

// Character / binary value codecs. Fixed text fields are space-padded on the
// right; fixed binary fields are NUL-padded on the right.

// ASCII is plain ASCII text (zero-copy decode).
type ascii struct{}

func (ascii) DecodeBody(body []byte, _ int, _ *iso8583.FieldDef) (iso8583.Value, error) {
	return iso8583.StringValue(body), nil
}
func (ascii) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	b, err := padFixed(v.Bytes(), def, ' ', false)
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}
func (ascii) Kind() iso8583.Kind { return iso8583.KindString }
func (ascii) Name() string       { return "char.ascii" }

// ebcdicChar transcodes EBCDIC text through a code-page table. Decode allocates
// (the documented EBCDIC transcoding cost).
type ebcdicChar struct {
	tbl  *ebcdic.Table
	name string
}

func (c ebcdicChar) DecodeBody(body []byte, _ int, _ *iso8583.FieldDef) (iso8583.Value, error) {
	return iso8583.StringValue(c.tbl.Decode(body)), nil
}
func (c ebcdicChar) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	ascii, err := padFixed(v.Bytes(), def, ' ', false)
	if err != nil {
		return nil, err
	}
	return append(dst, c.tbl.Encode(ascii)...), nil
}
func (ebcdicChar) Kind() iso8583.Kind { return iso8583.KindString }
func (c ebcdicChar) Name() string     { return c.name }

// utf8Char is UTF-8 text (zero-copy decode; an Isopace extra jPOS lacks).
type utf8Char struct{}

func (utf8Char) DecodeBody(body []byte, _ int, _ *iso8583.FieldDef) (iso8583.Value, error) {
	return iso8583.StringValue(body), nil
}
func (utf8Char) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	b, err := padFixed(v.Bytes(), def, ' ', false)
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}
func (utf8Char) Kind() iso8583.Kind { return iso8583.KindString }
func (utf8Char) Name() string       { return "char.utf8" }

// binaryRaw is raw octets (zero-copy decode), e.g. track data or an echo field.
type binaryRaw struct{}

func (binaryRaw) DecodeBody(body []byte, _ int, _ *iso8583.FieldDef) (iso8583.Value, error) {
	return iso8583.BytesValue(body), nil
}
func (binaryRaw) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	b, err := padFixed(v.Bytes(), def, 0x00, false)
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}
func (binaryRaw) Kind() iso8583.Kind { return iso8583.KindBytes }
func (binaryRaw) Name() string       { return "b.raw" }

// Catalog singletons.
var (
	ASCII      iso8583.FieldCodec = ascii{}
	EBCDIC037  iso8583.FieldCodec = ebcdicChar{tbl: ebcdic.CP037, name: "char.ebcdic.cp037"}
	EBCDIC1047 iso8583.FieldCodec = ebcdicChar{tbl: ebcdic.CP1047, name: "char.ebcdic.cp1047"}
	UTF8       iso8583.FieldCodec = utf8Char{}
	BINARY     iso8583.FieldCodec = binaryRaw{}
)
