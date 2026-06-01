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

// Package jsonio renders an ISO-8583 message to and from a schema-aware JSON
// document. It consumes the read-only iso8583.View, so it never re-parses the
// wire and adding it never touched the core or any codec (ARCHITECTURE.md §8).
// Field keys are the uniform FieldPath grammar; numerics stay strings to
// preserve leading zeros, amounts carry exact scale, and PAN masking is a render
// option (not baked into the model).
package jsonio

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/teqpace-services/isopace/iso8583"
)

// document is the JSON shape: an MTI plus a map of DE number -> field.
type document struct {
	MTI    string               `json:"mti,omitempty"`
	Fields map[string]fieldJSON `json:"fields"`
}

type fieldJSON struct {
	Name  string                     `json:"name,omitempty"`
	Value json.RawMessage            `json:"value,omitempty"` // scalar fields
	TLV   map[string]json.RawMessage `json:"tlv,omitempty"`   // composite (BER-TLV)
}

type amountJSON struct {
	Unscaled int64 `json:"unscaled"`
	Scale    uint8 `json:"scale"`
}

// options configure rendering.
type options struct {
	useNames bool
	maskPAN  bool
}

// Option mutates render options.
type Option func(*options)

// UseNames includes each field's schema name in the JSON.
func UseNames() Option { return func(o *options) { o.useNames = true } }

// MaskPAN masks the PAN (DE 2) in the output, keeping the leading six and
// trailing four digits. This is lossy and therefore not round-trippable.
func MaskPAN() Option { return func(o *options) { o.maskPAN = true } }

// Marshal renders a View to JSON.
func Marshal(v iso8583.View, opts ...Option) ([]byte, error) {
	var o options
	for _, f := range opts {
		f(&o)
	}
	schema := v.Schema()
	doc := document{Fields: make(map[string]fieldJSON)}
	if mti, ok := v.Get(0); ok {
		doc.MTI, _ = mti.String()
	}
	var ferr error
	for path, val := range v.Fields() {
		de := int(path.Head().N)
		def, _ := schema.Field(de)
		fj := fieldJSON{}
		if o.useNames && def != nil {
			fj.Name = def.Name
		}
		if val.Kind() == iso8583.KindComposite {
			tlv, err := renderTLV(val, def)
			if err != nil {
				ferr = err
				break
			}
			fj.TLV = tlv
		} else {
			raw, err := renderScalar(val, de, o)
			if err != nil {
				ferr = err
				break
			}
			fj.Value = raw
		}
		doc.Fields[strconv.Itoa(de)] = fj
	}
	if ferr != nil {
		return nil, ferr
	}
	return json.MarshalIndent(doc, "", "  ")
}

func renderScalar(v iso8583.Value, de int, o options) (json.RawMessage, error) {
	switch v.Kind() {
	case iso8583.KindAmount:
		d, err := v.Decimal()
		if err != nil {
			return nil, err
		}
		return json.Marshal(amountJSON{Unscaled: d.Unscaled, Scale: d.Scale})
	case iso8583.KindBytes:
		return json.Marshal(hexUpper(v.Bytes()))
	default: // KindString, KindNumeric
		s, err := v.String()
		if err != nil {
			return nil, err
		}
		if o.maskPAN && de == 2 {
			s = maskPAN(s)
		}
		return json.Marshal(s)
	}
}

func renderTLV(v iso8583.Value, def *iso8583.FieldDef) (map[string]json.RawMessage, error) {
	child, ok := v.Composite()
	if !ok {
		return nil, fmt.Errorf("jsonio: DE %s is not a composite", def.Path)
	}
	out := make(map[string]json.RawMessage)
	var ferr error
	for tag, tv := range child.TagSeq() {
		raw, err := renderScalar(tv, -1, options{})
		if err != nil {
			ferr = err
			break
		}
		out[tag] = raw
	}
	if ferr != nil {
		return nil, ferr
	}
	return out, nil
}

// Unmarshal parses a JSON document into a Message on the given schema.
func Unmarshal(data []byte, s *iso8583.Schema) (*iso8583.Message, error) {
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("jsonio: parse: %w", err)
	}
	m := iso8583.New(s)
	if doc.MTI != "" {
		if err := m.Set(0, doc.MTI); err != nil {
			return nil, err
		}
	}
	for deStr, fj := range doc.Fields {
		de, err := strconv.Atoi(deStr)
		if err != nil {
			return nil, fmt.Errorf("jsonio: field key %q is not a DE number", deStr)
		}
		def, ok := s.Field(de)
		if !ok {
			return nil, fmt.Errorf("jsonio: DE %d not in schema", de)
		}
		if fj.TLV != nil {
			for tag, raw := range fj.TLV {
				td, _ := def.Sub.LookupTag(tag)
				arg, err := argFromJSON(raw, kindOf(td))
				if err != nil {
					return nil, err
				}
				if err := m.SetS(strconv.Itoa(de)+"."+tag, arg); err != nil {
					return nil, err
				}
			}
			continue
		}
		if len(fj.Value) == 0 {
			continue
		}
		arg, err := argFromJSON(fj.Value, def.Kind)
		if err != nil {
			return nil, err
		}
		if err := m.Set(de, arg); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// argFromJSON turns a rendered JSON value back into a Set argument for the kind.
func argFromJSON(raw json.RawMessage, kind iso8583.Kind) (any, error) {
	switch kind {
	case iso8583.KindAmount:
		var a amountJSON
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, fmt.Errorf("jsonio: amount: %w", err)
		}
		return iso8583.NewDecimal(a.Unscaled, a.Scale), nil
	case iso8583.KindBytes:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("jsonio: bytes: %w", err)
		}
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("jsonio: bad hex: %w", err)
		}
		return b, nil
	default:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("jsonio: string: %w", err)
		}
		return s, nil
	}
}

func kindOf(def *iso8583.FieldDef) iso8583.Kind {
	if def == nil {
		return iso8583.KindBytes // unknown tag -> raw bytes
	}
	return def.Kind
}

func hexUpper(b []byte) string {
	s := hex.EncodeToString(b)
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'f' {
			out[i] = c - 'a' + 'A'
		}
	}
	return string(out)
}

func maskPAN(pan string) string {
	if len(pan) <= 10 {
		if len(pan) <= 4 {
			return pan
		}
		return repeat('*', len(pan)-4) + pan[len(pan)-4:]
	}
	return pan[:6] + repeat('*', len(pan)-10) + pan[len(pan)-4:]
}

func repeat(c byte, n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}
