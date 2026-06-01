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

// Package tlv implements the BER-TLV composite value codec used for EMV data
// (ISO-8583 DE 55) and other tag-length-value fields. Children are addressed by
// canonical hex tag under the uniform FieldPath grammar ("55.9F26"); the codec
// decodes a field body into a tag-addressed iso8583 sub-message and re-encodes
// it on demand.
package tlv

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/teqpace-services/isopace/iso8583"
)

// Errors returned by the BER-TLV codec.
var (
	ErrTruncated    = errors.New("tlv: truncated BER-TLV data")
	ErrIndefinite   = errors.New("tlv: unsupported indefinite/over-long length")
	ErrNotComposite = errors.New("tlv: value is not a composite")
)

type berTLV struct{}

// BERTLV is the BER-TLV composite codec (registry name "tlv.ber"). Wire it onto
// a composite field with its EMV sub-schema, e.g.
//
//	Composite(55, "EMV", emvSchema, lengthcodec.LLLVarASCII, iso8583.WithCodec(tlv.BERTLV))
var BERTLV iso8583.FieldCodec = berTLV{}

func (berTLV) Kind() iso8583.Kind { return iso8583.KindComposite }
func (berTLV) Name() string       { return "tlv.ber" }

// DecodeBody parses the BER-TLV body into a tag-addressed child message. Each
// tag's value is decoded with its sub-schema codec when defined, else retained
// as raw bytes so unknown tags still round-trip.
func (berTLV) DecodeBody(body []byte, _ int, def *iso8583.FieldDef) (iso8583.Value, error) {
	child := iso8583.NewTLV(def.Sub)
	for i := 0; i < len(body); {
		tag, val, next, err := readTLV(body, i)
		if err != nil {
			return iso8583.Value{}, err
		}
		i = next

		var tv iso8583.Value
		if td, ok := lookupTag(def, tag); ok && td.Codec != nil {
			dv, derr := td.Codec.DecodeBody(val, len(val), td)
			if derr != nil {
				return iso8583.Value{}, derr
			}
			tv = dv
		} else {
			tv = iso8583.BytesValue(val)
		}
		child.PutTag(tag, tv)
	}
	return iso8583.CompositeValue(body, child), nil
}

// EncodeBody serialises the child message back to BER-TLV, encoding each tag's
// value with its sub-schema codec when defined.
func (berTLV) EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	child, ok := v.Composite()
	if !ok {
		return nil, ErrNotComposite
	}
	var ferr error
	for tag, tv := range child.TagSeq() {
		tagBytes, err := hex.DecodeString(tag)
		if err != nil {
			ferr = err
			break
		}
		var val []byte
		if td, ok := lookupTag(def, tag); ok && td.Codec != nil {
			if val, err = td.Codec.EncodeBody(nil, tv, td); err != nil {
				ferr = err
				break
			}
		} else {
			val = tv.Bytes()
		}
		dst = append(dst, tagBytes...)
		dst = appendBERLen(dst, len(val))
		dst = append(dst, val...)
	}
	if ferr != nil {
		return nil, ferr
	}
	return dst, nil
}

func lookupTag(def *iso8583.FieldDef, tag string) (*iso8583.FieldDef, bool) {
	if def == nil || def.Sub == nil {
		return nil, false
	}
	return def.Sub.LookupTag(tag)
}

// readTLV parses one tag-length-value triplet starting at b[i], returning the
// canonical uppercase-hex tag, the value bytes, and the offset just past the
// triplet.
func readTLV(b []byte, i int) (tag string, val []byte, next int, err error) {
	if i >= len(b) {
		return "", nil, 0, ErrTruncated
	}
	start := i
	b0 := b[i]
	i++
	if b0&0x1F == 0x1F { // multi-byte tag: continue while high bit set
		for {
			if i >= len(b) {
				return "", nil, 0, ErrTruncated
			}
			c := b[i]
			i++
			if c&0x80 == 0 {
				break
			}
		}
	}
	tag = strings.ToUpper(hex.EncodeToString(b[start:i]))

	if i >= len(b) {
		return "", nil, 0, ErrTruncated
	}
	l0 := b[i]
	i++
	length := int(l0)
	if l0&0x80 != 0 {
		nb := int(l0 & 0x7F)
		if nb == 0 || nb > 4 {
			return "", nil, 0, ErrIndefinite
		}
		if i+nb > len(b) {
			return "", nil, 0, ErrTruncated
		}
		length = 0
		for k := 0; k < nb; k++ {
			length = length<<8 | int(b[i])
			i++
		}
	}
	if length < 0 || i+length > len(b) {
		return "", nil, 0, ErrTruncated
	}
	return tag, b[i : i+length], i + length, nil
}

// appendBERLen appends a definite-form BER length for n bytes.
func appendBERLen(dst []byte, n int) []byte {
	switch {
	case n < 0x80:
		return append(dst, byte(n))
	case n <= 0xFF:
		return append(dst, 0x81, byte(n))
	case n <= 0xFFFF:
		return append(dst, 0x82, byte(n>>8), byte(n))
	default:
		return append(dst, 0x83, byte(n>>16), byte(n>>8), byte(n))
	}
}
