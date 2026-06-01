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

package iso8583

import (
	"errors"
	"fmt"
	"strings"
)

// defaultMTILen is the wire width assumed for an MTI when none is configured;
// four characters for text encodings (DE 0).
const defaultMTILen = 4

// SchemaBuilder assembles a Schema. All validation is deferred to Build, which
// reports every problem at once. A builder is not safe for concurrent use; the
// Schema it produces is.
type SchemaBuilder struct {
	id     string
	mti    *FieldDef
	bitmap BitmapSpec
	defs   map[int]*FieldDef
	tags   map[string]*FieldDef
	order  []string // tag insertion order, for stable TLV iteration
	isTLV  bool
	errs   []error
}

// NewSchema starts a new schema builder with the given identifier.
func NewSchema(id string) *SchemaBuilder {
	return &SchemaBuilder{
		id:   id,
		defs: make(map[int]*FieldDef),
		tags: make(map[string]*FieldDef),
	}
}

// MTI sets the message-type-indicator value codec. The MTI is a fixed field of
// defaultMTILen wire bytes (DE 0).
func (b *SchemaBuilder) MTI(c FieldCodec) *SchemaBuilder {
	b.mti = &FieldDef{
		Path:   DE(0),
		Name:   "MTI",
		Kind:   kindOf(c),
		Codec:  c,
		MaxLen: defaultMTILen,
	}
	return b
}

// Bitmap sets the bitmap encoding spec.
func (b *SchemaBuilder) Bitmap(spec BitmapSpec) *SchemaBuilder {
	b.bitmap = spec
	return b
}

// Field wires a DE to a (value codec, length codec) pair plus options. The two
// codecs compose orthogonally; pass a nil length codec for a fixed field.
func (b *SchemaBuilder) Field(de int, name string, value FieldCodec, length LengthCodec, opts ...FieldOpt) *SchemaBuilder {
	d := &FieldDef{
		Path:   DE(de),
		Name:   name,
		Kind:   kindOf(value),
		Codec:  value,
		Length: length,
	}
	applyOpts(d, opts)
	if _, dup := b.defs[de]; dup {
		b.errs = append(b.errs, fmt.Errorf("DE %d defined more than once", de))
	}
	b.defs[de] = d
	return b
}

// Composite wires a DE to a nested sub-schema (subfield group or BER-TLV) with
// its own length prefix. Composite fields carry no value codec; the engine uses
// the Sub schema to decode children.
func (b *SchemaBuilder) Composite(de int, name string, sub *Schema, length LengthCodec, opts ...FieldOpt) *SchemaBuilder {
	d := &FieldDef{
		Path:   DE(de),
		Name:   name,
		Kind:   KindComposite,
		Length: length,
		Sub:    sub,
	}
	applyOpts(d, opts)
	if _, dup := b.defs[de]; dup {
		b.errs = append(b.errs, fmt.Errorf("DE %d defined more than once", de))
	}
	b.defs[de] = d
	return b
}

// Tag wires a BER-TLV child addressed by canonical hex tag (presence by tag,
// not a positional bitmap). It marks the schema as a TLV composite (C4).
func (b *SchemaBuilder) Tag(tag, name string, value FieldCodec, opts ...FieldOpt) *SchemaBuilder {
	key := strings.ToUpper(tag)
	d := &FieldDef{
		Path:  FieldPath{}.Push(PathElem{N: -1, Tag: key}),
		Name:  name,
		Kind:  kindOf(value),
		Codec: value,
	}
	applyOpts(d, opts)
	if _, dup := b.tags[key]; dup {
		b.errs = append(b.errs, fmt.Errorf("tag %s defined more than once", key))
	} else {
		b.order = append(b.order, key)
	}
	b.tags[key] = d
	b.isTLV = true
	return b
}

// Build validates the assembled schema and returns it, or an aggregate error.
// Per C1 it takes no registry: codecs arrive already resolved.
func (b *SchemaBuilder) Build() (*Schema, error) {
	errs := append([]error(nil), b.errs...)

	if b.id == "" {
		errs = append(errs, errors.New("schema id is empty"))
	}

	maxDE := 0
	for de, d := range b.defs {
		if de < 1 || de > maxDEIndex {
			errs = append(errs, fmt.Errorf("DE %d out of range 1..%d", de, maxDEIndex))
			continue
		}
		if de > maxDE {
			maxDE = de
		}
		errs = append(errs, validateDef(de, d)...)
	}
	for _, key := range b.order {
		errs = append(errs, validateTagDef(key, b.tags[key])...)
	}

	if !b.isTLV {
		if b.bitmap.Codec == nil {
			errs = append(errs, errors.New("schema has no bitmap codec"))
		}
		if b.bitmap.Levels < 1 || b.bitmap.Levels > 3 {
			errs = append(errs, fmt.Errorf("bitmap levels %d out of range 1..3", b.bitmap.Levels))
		}
		if b.mti == nil {
			errs = append(errs, errors.New("schema has no MTI codec"))
		}
	}

	if len(errs) > 0 {
		return nil, &SchemaError{ID: b.id, Errs: errs}
	}

	defs := make([]*FieldDef, maxDE+1)
	for de, d := range b.defs {
		defs[de] = d
	}
	return &Schema{
		id:     b.id,
		mti:    b.mti,
		bitmap: b.bitmap,
		defs:   defs,
		tags:   b.tags,
		maxDE:  maxDE,
		isTLV:  b.isTLV,
	}, nil
}

// MustBuild builds the schema and panics on error; for package-level profiles.
func (b *SchemaBuilder) MustBuild() *Schema {
	s, err := b.Build()
	if err != nil {
		panic(err)
	}
	return s
}

func validateDef(de int, d *FieldDef) []error {
	var errs []error
	if d.Sub == nil && d.Codec == nil {
		errs = append(errs, fmt.Errorf("DE %d has no value codec", de))
	}
	if d.Length == nil && d.Sub == nil && d.MaxLen <= 0 {
		errs = append(errs, fmt.Errorf("DE %d is fixed-length but MaxLen <= 0", de))
	}
	if d.Kind == KindComposite && d.Sub == nil {
		errs = append(errs, fmt.Errorf("DE %d is composite but has no sub-schema", de))
	}
	return errs
}

func validateTagDef(key string, d *FieldDef) []error {
	if d.Codec == nil && d.Sub == nil {
		return []error{fmt.Errorf("tag %s has no value codec", key)}
	}
	return nil
}

// clone returns a deep copy of d with independent backing arrays for its mutable
// slice fields (Validate, Required.MTIClasses). Derive/Override overlays mutate
// the copy, so appending to a shared slice can never corrupt a base schema's
// definitions (see TestDeriveDoesNotAliasBaseValidators).
func (d *FieldDef) clone() *FieldDef {
	cp := *d
	if d.Validate != nil {
		cp.Validate = append([]Validator(nil), d.Validate...)
	}
	if d.Required.MTIClasses != nil {
		cp.Required.MTIClasses = append([]string(nil), d.Required.MTIClasses...)
	}
	return &cp
}

// Derive clones the schema's definitions into a new builder so a card-scheme
// overlay can be expressed as deltas (Override/Field) over a base profile.
func (s *Schema) Derive(id string) *SchemaBuilder {
	b := NewSchema(id)
	b.bitmap = s.bitmap
	b.isTLV = s.isTLV
	if s.mti != nil {
		b.mti = s.mti.clone()
	}
	for de, d := range s.defs {
		if d == nil {
			continue
		}
		b.defs[de] = d.clone()
	}
	for k, d := range s.tags {
		b.tags[k] = d.clone()
		b.order = append(b.order, k)
	}
	return b
}

// Override modifies an existing DE definition with the given options. It records
// an error if the DE is not defined.
func (b *SchemaBuilder) Override(de int, opts ...FieldOpt) *SchemaBuilder {
	d, ok := b.defs[de]
	if !ok {
		b.errs = append(b.errs, fmt.Errorf("Override of undefined DE %d", de))
		return b
	}
	applyOpts(d, opts)
	return b
}

// FieldOpt configures a FieldDef during Field/Composite/Tag/Override.
type FieldOpt func(*FieldDef)

func applyOpts(d *FieldDef, opts []FieldOpt) {
	for _, o := range opts {
		o(d)
	}
}

// MaxLen sets the maximum value length (digits/chars/octets, or wire bytes for
// a fixed field).
func MaxLen(n int) FieldOpt { return func(d *FieldDef) { d.MaxLen = n } }

// Pad sets fixed-field padding.
func Pad(p PadRule) FieldOpt { return func(d *FieldDef) { d.Pad = p } }

// Optional marks a field as not required (the default).
func Optional() FieldOpt {
	return func(d *FieldDef) { d.Required = RequiredRule{Mode: RequiredOptional} }
}

// MustExist marks a field as always required.
func MustExist() FieldOpt {
	return func(d *FieldDef) { d.Required = RequiredRule{Mode: RequiredAlways} }
}

// RequiredOn marks a field required for the given MTI classes.
func RequiredOn(mtiClasses ...string) FieldOpt {
	return func(d *FieldDef) {
		d.Required = RequiredRule{Mode: RequiredOnMTI, MTIClasses: append([]string(nil), mtiClasses...)}
	}
}

// Validate attaches ordered field-level validators.
func Validate(v ...Validator) FieldOpt {
	return func(d *FieldDef) { d.Validate = append(d.Validate, v...) }
}

// WithCodec replaces a field's value codec (used by Derive/Override overlays).
func WithCodec(c FieldCodec) FieldOpt {
	return func(d *FieldDef) {
		d.Codec = c
		d.Kind = kindOf(c)
	}
}

// WithLength replaces a field's length codec.
func WithLength(l LengthCodec) FieldOpt { return func(d *FieldDef) { d.Length = l } }

// WithComposite replaces a field's sub-schema (and marks it composite).
func WithComposite(sub *Schema) FieldOpt {
	return func(d *FieldDef) {
		d.Sub = sub
		d.Kind = KindComposite
	}
}

// kindOf returns a codec's kind, or KindInvalid for a nil codec.
func kindOf(c FieldCodec) Kind {
	if c == nil {
		return KindInvalid
	}
	return c.Kind()
}

// SchemaError aggregates build-time schema validation failures.
type SchemaError struct {
	ID   string
	Errs []error
}

func (e *SchemaError) Error() string {
	var b strings.Builder
	b.WriteString("iso8583: schema ")
	b.WriteString(e.ID)
	b.WriteString(" invalid: ")
	for i, er := range e.Errs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(er.Error())
	}
	return b.String()
}

func (e *SchemaError) Unwrap() []error { return e.Errs }
