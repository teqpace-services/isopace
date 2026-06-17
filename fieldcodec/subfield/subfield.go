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

// Package subfield implements the positional subfield-packager composite value
// codec: a field whose body is itself a bitmap followed by positional
// subfields, the way ISO-8583 DE 127 (reserved-private use) is commonly carried
// — a nested mini-message addressed under the uniform path grammar ("127.2",
// "127.22"). It is the bitmap-addressed counterpart of fieldcodec/tlv (which is
// tag-addressed): the container's length prefix delimits the body, a sub-bitmap
// selects present subfields, and each subfield is packed by its own
// (value, length) codec pair from a headerless sub-schema.
package subfield

import (
	"errors"

	"github.com/teqpace-services/isopace/iso8583"
)

// ErrNotComposite is returned when EncodeBody is handed a non-composite value.
var ErrNotComposite = errors.New("subfield: value is not a composite")

type packager struct{}

// Packager is the positional subfield-packager composite codec (registry name
// "subfield.iso"). Wire it onto a composite DE whose Sub is a headerless
// positional schema (a bitmap plus subfield FieldDefs), e.g.
//
//	Composite(127, "Reserved Private Use", sub127,
//	    lengthcodec.LLLLLLVarASCII, iso8583.WithCodec(subfield.Packager))
//
// The sub-schema must be built with SchemaBuilder.Headerless() (no MTI); the
// body is exactly its bitmap followed by its present subfields.
var Packager iso8583.FieldCodec = packager{}

func (packager) Kind() iso8583.Kind { return iso8583.KindComposite }
func (packager) Name() string       { return "subfield.iso" }

// DecodeBody parses the subfield body (bitmap + positional subfields) into a
// child message via the headerless sub-schema's engine codec, so all positional
// decoding — length prefixes, packed widths, nested composites — is the same
// tested path the top-level message uses.
func (packager) DecodeBody(body []byte, _ int, def *iso8583.FieldDef) (iso8583.Value, error) {
	if def == nil || def.Sub == nil {
		return iso8583.Value{}, ErrNotComposite
	}
	child, err := iso8583.NewCodec(def.Sub).Unmarshal(body)
	if err != nil {
		return iso8583.Value{}, err
	}
	return iso8583.CompositeValue(body, child), nil
}

// EncodeBody serialises the child message back to bitmap + subfields. The
// engine's outer length codec then prefixes the returned body with the
// container length.
func (packager) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	if def == nil || def.Sub == nil {
		return nil, ErrNotComposite
	}
	child, ok := v.Composite()
	if !ok {
		return nil, ErrNotComposite
	}
	body, err := iso8583.NewCodec(def.Sub).Marshal(child, nil)
	if err != nil {
		return nil, err
	}
	return append(dst, body...), nil
}
