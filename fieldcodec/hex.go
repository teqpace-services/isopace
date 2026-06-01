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

package fieldcodec

import (
	"encoding/hex"

	"github.com/teqpace-services/isopace/iso8583"
)

// hexBinary is a binary value carried on the wire as ASCII hex: each octet of
// the model value is two hex characters on the wire, so a logical length of N
// octets occupies 2N wire bytes. It is the hex-text counterpart of b.raw, which
// writes the octets verbatim. The model Value is KindBytes — callers Set/Get raw
// octets and never see the hex transport.
type hexBinary struct{}

// DecodeBody parses the 2N-char hex body into N octets. units is the logical
// octet count (FieldDef.MaxLen for a fixed field, or the length prefix value).
func (hexBinary) DecodeBody(body []byte, _ int, _ *iso8583.FieldDef) (iso8583.Value, error) {
	out := make([]byte, len(body)/2)
	if _, err := hex.Decode(out, body); err != nil {
		return iso8583.Value{}, ErrBadHex
	}
	return iso8583.BytesValue(out), nil
}

// EncodeBody renders the octet value as uppercase ASCII hex, NUL-padding the
// octets to a fixed field's width before encoding (variable fields are emitted
// as-is).
func (hexBinary) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	octets, err := padFixed(v.Bytes(), def, 0x00, false)
	if err != nil {
		return nil, err
	}
	enc := make([]byte, hex.EncodedLen(len(octets)))
	hex.Encode(enc, octets)
	return append(dst, upperHex(enc)...), nil
}

func (hexBinary) Kind() iso8583.Kind { return iso8583.KindBytes }
func (hexBinary) Name() string       { return "b.hex" }

// BodyBytes reports the wire span: two hex chars per logical octet.
func (hexBinary) BodyBytes(units int) int { return units * 2 }

// HexBINARY is the ASCII-hex binary value codec (registry name "b.hex").
var HexBINARY iso8583.FieldCodec = hexBinary{}
