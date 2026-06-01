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

package packager

import (
	"strconv"

	"github.com/teqpace-services/isopace/fieldcodec"
	"github.com/teqpace-services/isopace/fieldcodec/subfield"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/lengthcodec"
)

// Concrete packagers (zone, fields, switch) are assembled directly from the
// catalog: unlike the representation-independent iso87/iso93 directories — which
// swap whole codec sets via a rep — each field here PINS one value codec and one
// length discipline, the two orthogonal axes of the wire engine. A packager is
// therefore just a table of (DE, name, value, length, max) rows, emitted two
// ways from the same description: a programmatic *iso8583.Schema (the schema
// methods) and the declarative JSON the generic loader consumes (the config
// methods). Sharing one table keeps the two forms from drifting.

// The value codecs the concrete packagers draw from (the value axis).
var (
	numeric = fieldcodec.NumASCII  // ASCII decimal digits, left zero-padded
	textual = fieldcodec.ASCII     // ASCII text, right space-padded
	binary  = fieldcodec.BINARY    // raw octets
	hexBin  = fieldcodec.HexBINARY // octets carried as ASCII hex (wire = 2x length)
)

// The length disciplines (the length axis). fixedWidth is the nil prefix: a
// fixed-width field whose span comes from its max. The llVar family is the
// LL..LLLLLL ASCII variable-length prefix.
var (
	fixedWidth iso8583.LengthCodec // nil: fixed width, no prefix
	llVar      = lengthcodec.LLVarASCII
	lllVar     = lengthcodec.LLLVarASCII
	llllVar    = lengthcodec.LLLLVarASCII
	lllllVar   = lengthcodec.LLLLLVarASCII
	llllllVar  = lengthcodec.LLLLLLVarASCII
)

// field pins one data element to a value codec and a length discipline.
type field struct {
	de     int
	name   string
	value  iso8583.FieldCodec
	length iso8583.LengthCodec // nil == fixed width
	max    int
}

// subPackager is a nested subfield group (e.g. DE 127): a headerless sub-bitmap
// plus positional subfields, addressed as 127.N. name is the container DE's name.
type subPackager struct {
	id     string
	name   string
	bitmap iso8583.BitmapCodec
	fields []field
}

// concretePackager is a full packager profile: an MTI codec, a message bitmap,
// positional fields, and an optional nested subfield group at DE 127.
type concretePackager struct {
	id     string
	mti    iso8583.FieldCodec
	bitmap iso8583.BitmapCodec
	fields []field
	sub127 *subPackager
}

// bitmapLevels is the level count the concrete packagers use: primary plus
// secondary, so fields and subfields up to 128 are addressable. The continuation
// bit governs whether the secondary half is actually emitted, so a message with
// only fields below 65 still serialises to a single 8-byte / 16-hex word.
const bitmapLevels = 2

// schema assembles the programmatic *iso8583.Schema.
func (p concretePackager) schema() *iso8583.Schema {
	b := iso8583.NewSchema(p.id).
		MTI(p.mti).
		Bitmap(iso8583.BitmapSpec{Codec: p.bitmap, Levels: bitmapLevels})
	for _, f := range p.fields {
		b.Field(f.de, f.name, f.value, f.length, iso8583.MaxLen(f.max))
	}
	if p.sub127 != nil {
		b.Composite(127, p.sub127.name, p.sub127.schema(),
			llllllVar, iso8583.WithCodec(subfield.Packager))
	}
	return b.MustBuild()
}

// schema assembles the headerless subfield-group sub-schema.
func (s subPackager) schema() *iso8583.Schema {
	b := iso8583.NewSchema(s.id).
		Headerless().
		Bitmap(iso8583.BitmapSpec{Codec: s.bitmap, Levels: bitmapLevels})
	for _, f := range s.fields {
		b.Field(f.de, f.name, f.value, f.length, iso8583.MaxLen(f.max))
	}
	return b.MustBuild()
}

// config renders the packager as the declarative document the generic loader
// consumes, so the same profile is also available as an editable JSON file.
func (p concretePackager) config() schemaConfig {
	cfg := schemaConfig{
		ID:     p.id,
		MTI:    p.mti.Name(),
		Bitmap: bitmapConfig{Codec: p.bitmap.Name(), Levels: bitmapLevels},
		Fields: make(map[string]fieldConfig, len(p.fields)+1),
	}
	for _, f := range p.fields {
		cfg.Fields[strconv.Itoa(f.de)] = f.config()
	}
	if p.sub127 != nil {
		cfg.Composites = map[string]compositeConfig{p.sub127.id: p.sub127.config()}
		cfg.Fields["127"] = fieldConfig{
			Name:      p.sub127.name,
			Length:    llllllVar.Name(),
			Composite: p.sub127.id,
		}
	}
	return cfg
}

// config renders the subfield group as a "subfields" composite document.
func (s subPackager) config() compositeConfig {
	bm := bitmapConfig{Codec: s.bitmap.Name(), Levels: bitmapLevels}
	cc := compositeConfig{
		Type:   "subfields",
		Bitmap: &bm,
		Fields: make(map[string]fieldConfig, len(s.fields)),
	}
	for _, f := range s.fields {
		cc.Fields[strconv.Itoa(f.de)] = f.config()
	}
	return cc
}

// config maps one field to its declarative form, naming each axis by its
// registry key.
func (f field) config() fieldConfig {
	return fieldConfig{Name: f.name, Codec: f.value.Name(), Length: lenName(f.length), Max: f.max}
}

// lenName is the registry key for a length discipline; a nil prefix is fixed.
func lenName(l iso8583.LengthCodec) string {
	if l == nil {
		return "len.fixed"
	}
	return l.Name()
}
