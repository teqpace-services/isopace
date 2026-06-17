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

// Package xmlio renders an ISO-8583 message to and from the isomsg XML
// interchange format — the same <isomsg>/<field> shape spoken by jPOS's
// XMLChannel/XMLPackager — so an Isopace endpoint can exchange messages with
// existing XML toolchains and human-readable transcripts. Like the other render
// packages it consumes only the read-only iso8583.View, so it never re-parses
// the wire and adding it never touched the core or any codec (ARCHITECTURE.md
// §8). The wire-byte equivalent of a jPOS packager is, in Isopace, the schema;
// this package is the XML projection of a message, not a binary packer.
//
// Shape:
//
//	<isomsg>
//	  <field id="0"  value="0200"/>
//	  <field id="2"  value="4111111111111111"/>
//	  <field id="4"  value="10.99"/>
//	  <field id="52" value="A1B2C3D4..." type="binary"/>
//	  <isomsg id="48">
//	    <field id="1" value="..."/>
//	  </isomsg>
//	</isomsg>
//
// DE 0 is the MTI. Scalar text/numeric fields carry their canonical value in the
// value attribute; binary fields carry uppercase hex and type="binary"; amounts
// carry their exact decimal string, so scale survives the round trip. Positional
// subfield groups (e.g. DE 48, 127) nest as <isomsg id="N"> with positional
// child ids. BER-TLV composites (e.g. DE 55) also nest, addressing children by
// their hex tag — an Isopace extension over jPOS, which has no native TLV model.
package xmlio

import (
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/teqpace-services/isopace/iso8583"
)

// options configure rendering.
type options struct {
	useNames bool
	maskPAN  bool
}

// Option mutates render options.
type Option func(*options)

// UseNames adds each field's schema name as a name attribute. Names are
// advisory: Unmarshal ignores them and reconstructs purely from id + value.
func UseNames() Option { return func(o *options) { o.useNames = true } }

// MaskPAN masks the PAN (DE 2) in the output, keeping the leading six and
// trailing four digits. This is lossy and therefore not round-trippable.
func MaskPAN() Option { return func(o *options) { o.maskPAN = true } }

// indentStep is one level of indentation in the rendered document.
const indentStep = "  "

// Marshal renders a View to an isomsg XML document. The output carries no XML
// declaration so it can be framed directly on an XML channel (see link.ISOXML).
func Marshal(v iso8583.View, opts ...Option) ([]byte, error) {
	var o options
	for _, f := range opts {
		f(&o)
	}
	schema := v.Schema()

	var b strings.Builder
	b.WriteString("<isomsg>\n")

	// DE 0 is the MTI; emit it first, mirroring the ascending-id order jPOS uses.
	if mti, ok := v.Get(0); ok {
		name := ""
		if d := schema.MTIDef(); d != nil {
			name = d.Name
		}
		if err := writeScalar(&b, indentStep, "0", 0, name, mti, &o); err != nil {
			return nil, err
		}
	}
	for path, val := range v.Fields() {
		de := int(path.Head().N)
		if err := writeField(&b, indentStep, strconv.Itoa(de), de, schema, val, &o); err != nil {
			return nil, err
		}
	}

	b.WriteString("</isomsg>\n")
	return []byte(b.String()), nil
}

// writeField renders one field, descending into a nested <isomsg> for composites.
func writeField(b *strings.Builder, indent, id string, de int, schema *iso8583.Schema, val iso8583.Value, o *options) error {
	if child, ok := val.Composite(); ok {
		return writeComposite(b, indent, id, de, schema, child, o)
	}
	return writeScalar(b, indent, id, de, fieldName(schema, de), val, o)
}

// writeComposite renders a composite DE as a nested <isomsg id="N">, recursing
// into its children: BER-TLV tags for a TLV sub-schema, otherwise positional
// subfields.
func writeComposite(b *strings.Builder, indent, id string, de int, schema *iso8583.Schema, child *iso8583.Message, o *options) error {
	b.WriteString(indent)
	b.WriteString(`<isomsg`)
	writeAttr(b, "id", id)
	if o.useNames {
		if name := fieldName(schema, de); name != "" {
			writeAttr(b, "name", name)
		}
	}
	b.WriteString(">\n")

	sub := child.Schema()
	inner := indent + indentStep
	if sub != nil && sub.IsTLV() {
		for tag, cv := range child.TagSeq() {
			def, _ := sub.LookupTag(tag)
			if err := writeScalar(b, inner, tag, -1, defName(def), cv, o); err != nil {
				return err
			}
		}
	} else {
		for p, cv := range child.Fields() {
			cde := int(p.Head().N)
			if err := writeField(b, inner, strconv.Itoa(cde), cde, sub, cv, o); err != nil {
				return err
			}
		}
	}

	b.WriteString(indent)
	b.WriteString("</isomsg>\n")
	return nil
}

// writeScalar renders one scalar field element. de is the top-level DE for PAN
// masking (-1 for nested children, which are never masked here).
func writeScalar(b *strings.Builder, indent, id string, de int, name string, val iso8583.Value, o *options) error {
	b.WriteString(indent)
	b.WriteString(`<field`)
	writeAttr(b, "id", id)

	switch val.Kind() {
	case iso8583.KindBytes:
		writeAttr(b, "value", hexUpper(val.Bytes()))
		b.WriteString(` type="binary"`)
	case iso8583.KindAmount:
		d, err := val.Decimal()
		if err != nil {
			return fmt.Errorf("xmlio: field %s amount: %w", id, err)
		}
		writeAttr(b, "value", d.String())
	default: // KindString, KindNumeric
		s, err := val.String()
		if err != nil {
			return fmt.Errorf("xmlio: field %s value: %w", id, err)
		}
		if o.maskPAN && de == 2 {
			s = maskPAN(s)
		}
		writeAttr(b, "value", s)
	}

	if o.useNames && name != "" {
		writeAttr(b, "name", name)
	}
	b.WriteString("/>\n")
	return nil
}

// writeAttr writes a single, XML-escaped attribute: ` key="value"`.
func writeAttr(b *strings.Builder, key, val string) {
	b.WriteByte(' ')
	b.WriteString(key)
	b.WriteString(`="`)
	_ = xml.EscapeText(b, []byte(val)) // strings.Builder never errors
	b.WriteByte('"')
}

// --- parse ---

// xmlMsg is the recursive parse shape. The XMLName binding rejects any root that
// is not <isomsg>. Child <field> and <isomsg> elements collect into their slices
// regardless of document order (only re-encode order within a composite matters,
// and that is preserved by the field slice's document order).
type xmlMsg struct {
	XMLName xml.Name   `xml:"isomsg"`
	ID      string     `xml:"id,attr"`
	Fields  []xmlField `xml:"field"`
	Subs    []xmlMsg   `xml:"isomsg"`
}

type xmlField struct {
	ID    string `xml:"id,attr"`
	Value string `xml:"value,attr"`
	Type  string `xml:"type,attr"`
}

// Unmarshal parses an isomsg XML document into a Message on the given schema.
// Fields are routed by the schema's kind (and the type="binary" hint), so an
// amount recovers its exact scale and a binary field its octets.
func Unmarshal(data []byte, s *iso8583.Schema) (*iso8583.Message, error) {
	var root xmlMsg
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("xmlio: parse: %w", err)
	}
	m := iso8583.New(s)
	if err := apply(m, s, &root, ""); err != nil {
		return nil, err
	}
	return m, nil
}

// apply walks one <isomsg> level, setting scalar fields and recursing into
// nested sub-messages. prefix is the dotted path of the enclosing composite
// ("" at the top level), so a child is addressed as "48.1" or "55.9F26".
func apply(m *iso8583.Message, s *iso8583.Schema, msg *xmlMsg, prefix string) error {
	for _, f := range msg.Fields {
		if f.ID == "" {
			return fmt.Errorf("xmlio: <field> missing id attribute")
		}
		path := join(prefix, f.ID)
		arg, err := argFor(f, kindAt(s, path))
		if err != nil {
			return err
		}
		if err := m.SetS(path, arg); err != nil {
			return fmt.Errorf("xmlio: set %s: %w", path, err)
		}
	}
	for i := range msg.Subs {
		sub := &msg.Subs[i]
		if sub.ID == "" {
			return fmt.Errorf("xmlio: nested <isomsg> missing id attribute")
		}
		if err := apply(m, s, sub, join(prefix, sub.ID)); err != nil {
			return err
		}
	}
	return nil
}

// argFor turns a parsed field into the Set argument for its kind. The
// type="binary" hint wins (it round-trips bytes even for an unknown DE/tag);
// otherwise an amount is parsed to an exact Decimal and everything else stays a
// string so the value codec applies the right canonical form.
func argFor(f xmlField, kind iso8583.Kind) (any, error) {
	if f.Type == "binary" {
		b, err := hex.DecodeString(f.Value)
		if err != nil {
			return nil, fmt.Errorf("xmlio: field %s bad hex: %w", f.ID, err)
		}
		return b, nil
	}
	if kind == iso8583.KindAmount {
		d, err := parseDecimal(f.Value)
		if err != nil {
			return nil, fmt.Errorf("xmlio: field %s amount: %w", f.ID, err)
		}
		return d, nil
	}
	return f.Value, nil
}

// kindAt resolves the schema kind for a dotted path, defaulting to KindString
// for paths the schema does not define (unknown DEs / tags carry as text, or as
// bytes when the field is tagged type="binary").
func kindAt(s *iso8583.Schema, path string) iso8583.Kind {
	p, err := iso8583.ParsePath(path)
	if err != nil {
		return iso8583.KindString
	}
	def, ok := s.Lookup(p)
	if !ok || def == nil {
		return iso8583.KindString
	}
	return def.Kind
}

// --- helpers ---

func join(prefix, id string) string {
	if prefix == "" {
		return id
	}
	return prefix + "." + id
}

func fieldName(s *iso8583.Schema, de int) string {
	if s == nil {
		return ""
	}
	if def, ok := s.Field(de); ok {
		return def.Name
	}
	return ""
}

func defName(def *iso8583.FieldDef) string {
	if def == nil {
		return ""
	}
	return def.Name
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

// parseDecimal parses a plain decimal string into an exact Decimal, recovering
// the scale from the fractional digit count so "10.99" round-trips to
// {unscaled: 1099, scale: 2} and "1099" to {1099, 0}.
func parseDecimal(s string) (iso8583.Decimal, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	intPart, frac, _ := strings.Cut(s, ".")
	digits := intPart + frac
	if digits == "" {
		return iso8583.Decimal{}, fmt.Errorf("empty amount")
	}
	unscaled, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return iso8583.Decimal{}, err
	}
	if neg {
		unscaled = -unscaled
	}
	return iso8583.NewDecimal(unscaled, uint8(len(frac))), nil
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
