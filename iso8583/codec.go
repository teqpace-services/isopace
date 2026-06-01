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

import "strings"

// Codec drives a full message through a Schema. It is stateless and
// concurrency-safe; one instance serves all goroutines. Field codecs are
// pre-resolved on the Schema, so the hot path is an array index, not a registry
// lookup.
type Codec struct {
	schema *Schema
	opts   CodecOptions
}

// CodecOptions tunes engine behaviour.
type CodecOptions struct {
	Immutable bool // Unmarshal yields a sealed (copy-on-write) Message
	Strict    bool // fail on unknown DEs / length overruns vs. lenient skip
}

// CodecOption mutates CodecOptions.
type CodecOption func(*CodecOptions)

// Immutable makes Unmarshal yield a sealed message.
func Immutable() CodecOption { return func(o *CodecOptions) { o.Immutable = true } }

// Strict makes the engine fail on unknown DEs and length overruns.
func Strict() CodecOption { return func(o *CodecOptions) { o.Strict = true } }

// NewCodec builds a Codec for a schema.
func NewCodec(s *Schema, opts ...CodecOption) *Codec {
	var o CodecOptions
	for _, f := range opts {
		f(&o)
	}
	return &Codec{schema: s, opts: o}
}

// Schema returns the codec's schema.
func (c *Codec) Schema() *Schema { return c.schema }

// Unmarshal does a ZERO-COPY structural parse: it reads MTI + bitmap, then
// records each present field as a (offset,len) span into wire. Field bodies are
// NOT decoded; they decode lazily on first Get and memoise. wire is RETAINED by
// the Message (Values alias into it) — the caller must not mutate wire while the
// Message lives.
func (c *Codec) Unmarshal(wire []byte) (*Message, error) {
	s := c.schema
	m := newDecoded(s, wire, c.opts.Immutable)
	off := 0

	if s.mti != nil {
		bodyOff, bodyEnd, next, units, err := fieldBodySpan(s.mti, wire, off)
		if err != nil {
			return nil, &FieldError{Path: DE(0), Offset: off, Name: s.mti.Name, Err: err}
		}
		m.slots[0] = slot{present: true, fromSrc: true, wireOff: off, bodyOff: bodyOff, bodyEnd: bodyEnd, units: units}
		off = next
	}

	if s.bitmap.Codec == nil {
		return nil, &FieldError{Path: DE(1), Offset: off, Err: errUnknownDE}
	}
	bm, next, err := s.bitmap.Codec.ReadBitmap(wire, off, s.bitmap.Levels)
	if err != nil {
		return nil, &FieldError{Path: DE(1), Offset: off, Err: err}
	}
	m.bm = bm
	off = next

	var ferr error
	bm.Range(func(de int) bool {
		def, ok := s.Field(de)
		if !ok {
			// An unknown DE cannot be skipped: its length is undefined, so the
			// remaining stream is unparseable regardless of Strict.
			ferr = &FieldError{Path: DE(de), Offset: off, Err: errUnknownDE}
			return false
		}
		bodyOff, bodyEnd, nx, units, err := fieldBodySpan(def, wire, off)
		if err != nil {
			ferr = &FieldError{Path: DE(de), Offset: off, Name: def.Name, Err: err}
			return false
		}
		if de < len(m.slots) {
			m.slots[de] = slot{present: true, fromSrc: true, wireOff: off, bodyOff: bodyOff, bodyEnd: bodyEnd, units: units}
		}
		off = nx
		return true
	})
	if ferr != nil {
		return nil, ferr
	}
	return m, nil
}

// Marshal renders a Message to wire bytes, APPENDING to dst (reusing its
// capacity). The bitmap is DERIVED from present fields here (never hand-set).
//
// Fast path: if the message is unmodified since Unmarshal, the original src
// slice is returned VERBATIM — a store-and-forward hop pays zero re-encode or
// allocation cost. The returned slice then ALIASES src; callers needing an
// independent buffer must copy.
func (c *Codec) Marshal(m *Message, dst []byte) ([]byte, error) {
	if m.src != nil && !m.owned {
		return m.src, nil
	}
	s := c.schema
	scratch := make([]byte, 0, 64)
	var err error

	if s.mti != nil {
		dst, scratch, err = appendField(dst, scratch, s.mti, &m.slots[0], m.src)
		if err != nil {
			return nil, err
		}
	}

	var bm Bitmap
	for de := 1; de < len(m.slots); de++ {
		if m.slots[de].present {
			bm.Set(de)
		}
	}
	if s.bitmap.Codec == nil {
		return nil, &FieldError{Path: DE(1), Offset: -1, Err: errUnknownDE}
	}
	dst, err = s.bitmap.Codec.WriteBitmap(dst, bm, s.bitmap.Levels)
	if err != nil {
		return nil, &FieldError{Path: DE(1), Offset: -1, Err: err}
	}

	for de := 1; de < len(m.slots); de++ {
		sl := &m.slots[de]
		if !sl.present {
			continue
		}
		def, ok := s.Field(de)
		if !ok {
			return nil, &FieldError{Path: DE(de), Offset: -1, Err: errUnknownDE}
		}
		dst, scratch, err = appendField(dst, scratch, def, sl, m.src)
		if err != nil {
			return nil, err
		}
	}
	return dst, nil
}

// Validate runs every present field's validators plus required / RequiredOn(MTI)
// rules and returns ALL violations (non-fail-fast) as a *ValidationError, or
// nil.
func (c *Codec) Validate(m *Message) error {
	s := c.schema
	var vs []Violation

	mti := ""
	if v, ok := m.Get(0); ok {
		if str, err := v.String(); err == nil {
			mti = str
		}
	}

	for de := 1; de <= s.maxDE; de++ {
		def, ok := s.Field(de)
		if !ok {
			continue
		}
		present := m.Has(de)

		switch def.Required.Mode {
		case RequiredAlways:
			if !present {
				vs = append(vs, Violation{Path: def.Path, Offset: -1, Rule: "required", Msg: "field required but absent"})
			}
		case RequiredOnMTI:
			if !present && mtiInClasses(mti, def.Required.MTIClasses) {
				vs = append(vs, Violation{Path: def.Path, Offset: -1, Rule: "required", Msg: "field required for MTI but absent"})
			}
		}

		if !present {
			continue
		}
		v, ok := m.Get(de)
		if !ok {
			vs = append(vs, Violation{Path: def.Path, Offset: -1, Rule: "decode", Msg: "field present but could not be decoded"})
			continue
		}
		for _, rule := range def.Validate {
			if viol := rule.Validate(v, def); viol != nil {
				if viol.Path.IsZero() {
					viol.Path = def.Path
				}
				vs = append(vs, *viol)
			}
		}
	}

	if len(vs) > 0 {
		return &ValidationError{Violations: vs}
	}
	return nil
}

// fieldBodySpan reads the length prefix (if any) at off and returns the body
// span, the offset just past the field, and the logical unit count. A nil
// LengthCodec means a fixed field of MaxLen units. The unit count is converted
// to a wire byte span through the value codec's optional WidthCodec.
func fieldBodySpan(def *FieldDef, src []byte, off int) (bodyOff, bodyEnd, next, units int, err error) {
	if off < 0 || off > len(src) {
		return 0, 0, 0, 0, errTruncated
	}
	if def.Length == nil {
		units = def.MaxLen
		bodyOff = off
	} else {
		var afterPrefix int
		units, afterPrefix, err = def.Length.ReadLen(src, off, def)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if units < 0 || afterPrefix < off {
			return 0, 0, 0, 0, errBadLength
		}
		bodyOff = afterPrefix
	}
	bodyEnd = bodyOff + wireBytes(def, units)
	if bodyEnd > len(src) {
		return 0, 0, 0, 0, errTruncated
	}
	return bodyOff, bodyEnd, bodyEnd, units, nil
}

// wireBytes converts a logical unit count to the body's wire byte width using
// the value codec's optional WidthCodec (packed BCD: 2 units/byte); the default
// is 1 unit == 1 byte (and composites count bytes directly).
func wireBytes(def *FieldDef, units int) int {
	if wc, ok := def.Codec.(WidthCodec); ok {
		return wc.BodyBytes(units)
	}
	return units
}

// appendField appends one field (length prefix + body) to dst. A clean field is
// copied VERBATIM from src (prefix + body), preserving any packed/odd-length
// encoding exactly; a dirty or freshly-built field is re-encoded via its codec.
// scratch is reused across fields to avoid per-field allocation.
func appendField(dst, scratch []byte, def *FieldDef, sl *slot, src []byte) (outDst, outScratch []byte, err error) {
	if sl.fromSrc && !sl.dirty && src != nil {
		// Verbatim copy of the original prefix + body.
		dst = append(dst, src[sl.wireOff:sl.bodyEnd]...)
		return dst, scratch, nil
	}

	var body []byte
	if def.Codec != nil {
		scratch, err = def.Codec.EncodeBody(scratch[:0], sl.v, def)
		if err != nil {
			return dst, scratch, &FieldError{Path: def.Path, Offset: -1, Name: def.Name, Err: err}
		}
		body = scratch
	} else {
		// Composite or raw-set field with no value codec.
		body = sl.v.raw
	}

	if def.Length != nil {
		dst, err = def.Length.WriteLen(dst, encodeUnits(def, sl.v, body), def)
		if err != nil {
			return dst, scratch, &FieldError{Path: def.Path, Offset: -1, Name: def.Name, Err: err}
		}
	}
	dst = append(dst, body...)
	return dst, scratch, nil
}

// encodeUnits returns the logical unit count to write into the length prefix for
// a freshly-encoded body. For digit/char/byte values it is the canonical length
// (so an odd BCD digit count is preserved); for composites it is the body byte
// count.
func encodeUnits(def *FieldDef, v Value, body []byte) int {
	switch v.Kind() {
	case KindNumeric, KindString, KindBytes:
		return len(v.Bytes())
	case KindComposite:
		return len(body)
	case KindAmount:
		// Amounts are fixed-width in practice (no length prefix); fall back to
		// the canonical digit count if one is ever variable.
		if n := len(v.Bytes()); n > 0 {
			return n
		}
		return def.MaxLen
	default:
		return len(body)
	}
}

// mtiInClasses reports whether mti begins with any of the listed class prefixes
// (e.g. class "02" matches "0200" and "0210").
func mtiInClasses(mti string, classes []string) bool {
	for _, c := range classes {
		if strings.HasPrefix(mti, c) {
			return true
		}
	}
	return false
}
