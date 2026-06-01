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

// Package protobuf renders an ISO-8583 message to and from the protobuf binary
// wire format, keyed by data element: protobuf field number 1 carries the MTI
// and field number n (n >= 2) carries DE n, each as a length-delimited value.
// This is descriptor-compatible — a consumer's .proto declares `string f2 = 2;
// bytes f55 = 55;` etc. — without pulling in a protobuf runtime dependency, so
// the module stays stdlib-only. It consumes the read-only iso8583.View
// (ARCHITECTURE.md §8).
package protobuf

import (
	"encoding/binary"
	"errors"
	"strconv"

	"github.com/teqpace-services/isopace/iso8583"
)

// fieldMTI is the protobuf field number used for the MTI (DE 1 is the bitmap
// continuation indicator and never a data field, so field 1 is free).
const fieldMTI = 1

const wireLEN = 2 // protobuf length-delimited wire type

// Errors returned by the protobuf renderer.
var (
	ErrTruncated = errors.New("protobuf: truncated wire data")
	ErrWireType  = errors.New("protobuf: unsupported wire type (only length-delimited)")
	ErrFieldNum  = errors.New("protobuf: invalid field number")
)

// Marshal renders a View to protobuf wire bytes.
func Marshal(v iso8583.View) ([]byte, error) {
	schema := v.Schema()
	var dst []byte
	if mti, ok := v.Get(0); ok {
		s, err := mti.String()
		if err != nil {
			return nil, err
		}
		dst = appendField(dst, fieldMTI, []byte(s))
	}
	var ferr error
	for path, val := range v.Fields() {
		de := int(path.Head().N)
		def, _ := schema.Field(de)
		payload, err := encodeValue(val, def)
		if err != nil {
			ferr = err
			break
		}
		dst = appendField(dst, de, payload)
	}
	if ferr != nil {
		return nil, ferr
	}
	return dst, nil
}

func encodeValue(v iso8583.Value, def *iso8583.FieldDef) ([]byte, error) {
	switch v.Kind() {
	case iso8583.KindAmount:
		d, err := v.Decimal()
		if err != nil {
			return nil, err
		}
		return []byte(strconv.FormatInt(d.Unscaled, 10)), nil
	case iso8583.KindComposite:
		if def == nil || def.Codec == nil {
			return nil, errors.New("protobuf: composite field has no codec")
		}
		return def.Codec.EncodeBody(nil, v, def)
	default:
		return v.Bytes(), nil
	}
}

// Unmarshal parses protobuf wire bytes into a Message on the given schema.
func Unmarshal(data []byte, s *iso8583.Schema) (*iso8583.Message, error) {
	m := iso8583.New(s)
	for i := 0; i < len(data); {
		tag, n := binary.Uvarint(data[i:])
		if n <= 0 {
			return nil, ErrTruncated
		}
		i += n
		fieldNum := int(tag >> 3)
		if tag&7 != wireLEN {
			return nil, ErrWireType
		}
		if fieldNum < 1 {
			return nil, ErrFieldNum
		}
		ln, n := binary.Uvarint(data[i:])
		if n <= 0 {
			return nil, ErrTruncated
		}
		i += n
		if ln > uint64(len(data)-i) {
			return nil, ErrTruncated
		}
		payload := data[i : i+int(ln)]
		i += int(ln)

		if fieldNum == fieldMTI {
			if err := m.Set(0, string(payload)); err != nil {
				return nil, err
			}
			continue
		}
		def, ok := s.Field(fieldNum)
		if !ok {
			return nil, ErrFieldNum
		}
		if err := setValue(m, fieldNum, def, payload); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func setValue(m *iso8583.Message, de int, def *iso8583.FieldDef, payload []byte) error {
	switch def.Kind {
	case iso8583.KindAmount:
		n, err := strconv.ParseInt(string(payload), 10, 64)
		if err != nil {
			return err
		}
		return m.Set(de, iso8583.NewDecimal(n, def.Scale))
	case iso8583.KindComposite:
		if def.Codec == nil {
			return errors.New("protobuf: composite field has no codec")
		}
		v, err := def.Codec.DecodeBody(payload, len(payload), def)
		if err != nil {
			return err
		}
		return m.Set(de, v)
	case iso8583.KindBytes:
		return m.Set(de, append([]byte(nil), payload...))
	default:
		return m.Set(de, string(payload))
	}
}

// appendField appends one length-delimited protobuf field.
func appendField(dst []byte, fieldNum int, payload []byte) []byte {
	var hdr [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(hdr[:], uint64(fieldNum)<<3|wireLEN)
	dst = append(dst, hdr[:n]...)
	n = binary.PutUvarint(hdr[:], uint64(len(payload)))
	dst = append(dst, hdr[:n]...)
	return append(dst, payload...)
}
