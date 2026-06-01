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
	"strings"
	"sync"
)

// PadSide selects which end of a fixed field is padded.
type PadSide uint8

const (
	PadNone  PadSide = iota // no padding
	PadLeft                 // pad on the left (e.g. zero-fill numerics)
	PadRight                // pad on the right (e.g. space-fill text)
)

// PadRule describes fixed-field padding applied by value codecs.
type PadRule struct {
	Side PadSide
	Char byte
}

// RequiredMode classifies whether a field must be present.
type RequiredMode uint8

const (
	RequiredOptional RequiredMode = iota // may be absent
	RequiredAlways                       // must always be present
	RequiredOnMTI                        // required for the listed MTI classes
)

// RequiredRule captures a field's presence requirement.
type RequiredRule struct {
	Mode       RequiredMode
	MTIClasses []string // for RequiredOnMTI; matched against the message's MTI
}

// FieldDef defines one DE (or subfield/tag). It references codecs by interface
// (already resolved at Schema build time), never by name at runtime.
type FieldDef struct {
	Path     FieldPath
	Name     string       // "Primary Account Number"
	Kind     Kind         // canonical model kind
	Codec    FieldCodec   // VALUE codec
	Length   LengthCodec  // LENGTH-PREFIX codec; nil == fixed MaxLen wire bytes
	MaxLen   int          // max value length in logical units (digits/chars/octets); the wire byte span is derived via the codec's optional WidthCodec
	Scale    uint8        // decimal scale for KindAmount fields (minor-unit digits)
	Pad      PadRule      // fixed-field padding
	Validate []Validator  // ordered field-level rules
	Required RequiredRule // presence requirement
	Sub      *Schema      // non-nil for composites (DE 48/55/60/63…)
}

// BitmapSpec selects the bitmap wire encoding and supported levels.
type BitmapSpec struct {
	Codec  BitmapCodec // BitmapBinary | BitmapHex | BitmapEBCDIC
	Levels int         // 1=primary, 2=+secondary, 3=+tertiary
}

// Schema describes one message format. It is immutable after Build and safe for
// concurrent use across all goroutines and messages — built once, never copied
// per message.
type Schema struct {
	id     string
	mti    *FieldDef
	bitmap BitmapSpec
	defs   []*FieldDef          // indexed by DE (nil = undefined); len == maxDE+1
	tags   map[string]*FieldDef // BER-TLV children, keyed by canonical hex tag
	maxDE  int
	isTLV  bool     // composite addressed by tag rather than a positional bitmap
	binds  sync.Map // reflect.Type -> *bindPlan (struct-binding plans)
}

// ID returns the schema identifier, e.g. "iso87-a".
func (s *Schema) ID() string { return s.id }

// MaxDE returns the highest defined data element.
func (s *Schema) MaxDE() int { return s.maxDE }

// BitmapSpec returns the bitmap encoding spec.
func (s *Schema) BitmapSpec() BitmapSpec { return s.bitmap }

// MTIDef returns the MTI field definition, or nil for a TLV sub-schema.
func (s *Schema) MTIDef() *FieldDef { return s.mti }

// IsTLV reports whether this (sub-)schema addresses children by BER-TLV tag
// rather than a positional bitmap.
func (s *Schema) IsTLV() bool { return s.isTLV }

// Field returns the definition for data element de. DE 0 resolves to the MTI
// definition (stored separately) so the uniform path grammar addresses it too.
func (s *Schema) Field(de int) (*FieldDef, bool) {
	if de == 0 {
		return s.mti, s.mti != nil
	}
	if de < 0 || de >= len(s.defs) {
		return nil, false
	}
	d := s.defs[de]
	return d, d != nil
}

// tagDef returns the definition for a BER-TLV tag (canonical uppercase).
func (s *Schema) tagDef(tag string) (*FieldDef, bool) {
	d, ok := s.tags[tag]
	return d, ok
}

// LookupTag returns the FieldDef for a BER-TLV tag (matched case-insensitively),
// for composite codecs resolving wire tags against a TLV sub-schema.
func (s *Schema) LookupTag(tag string) (*FieldDef, bool) {
	d, ok := s.tags[strings.ToUpper(tag)]
	return d, ok
}

// Lookup resolves a full FieldPath, walking composites. A numeric element
// indexes defs; under a TLV sub-schema the element is matched as a hex tag.
func (s *Schema) Lookup(p FieldPath) (*FieldDef, bool) {
	if p.Len() == 0 {
		return nil, false
	}
	cur := s
	var def *FieldDef
	for i := 0; i < p.Len(); i++ {
		e := p.At(i)
		var ok bool
		if cur.isTLV || e.IsTag() {
			def, ok = cur.tagDef(tagKey(e))
		} else {
			def, ok = cur.Field(int(e.N))
		}
		if !ok {
			return nil, false
		}
		if i < p.Len()-1 {
			if def.Sub == nil {
				return nil, false
			}
			cur = def.Sub
		}
	}
	return def, true
}

// tagKey derives the canonical tag lookup key for an element: the hex tag when
// present, otherwise the numeric value rendered as-is.
func tagKey(e PathElem) string {
	if e.IsTag() {
		return e.Tag
	}
	return e.String()
}
