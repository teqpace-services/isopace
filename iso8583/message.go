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
	"fmt"
	"iter"
	"strconv"
	"strings"
)

// Message is a schema-driven, array-indexed sparse field set over a retained
// source buffer. While read-only every Value aliases the shared src, so a
// decoded Message is safe to publish to many goroutines with zero locking and
// zero copy. The first mutation flips the message to owned and clones only the
// touched field; everything else keeps aliasing src.
type Message struct {
	schema *Schema
	src    []byte // retained wire buffer; field Values alias INTO this (nil if built fresh)
	bm     Bitmap // derived presence map (data bits)
	slots  []slot // indexed by DE (len == schema.maxDE+1); no map lookup on hot path
	owned  bool   // true once any field is mutated (copy-on-write engaged)
	dirty  Bitmap // fields mutated since decode -> drives selective re-marshal
	sealed bool   // immutable view; mutation requires Clone()

	// tags holds BER-TLV / tag-addressed children for a TLV (schema.isTLV)
	// sub-message; tagOrder preserves emit order. nil for positional schemas.
	tags     map[string]tagSlot
	tagOrder []string
}

type slot struct {
	v       Value
	present bool // present per bitmap / set
	decoded bool // Value materialised (memoised) on first typed access
	dirty   bool // value replaced since decode (needs re-encode)
	fromSrc bool // body span [bodyOff,bodyEnd) into Message.src is valid
	units   int  // logical unit count (digits/chars/octets) for this field
	wireOff int  // absolute offset of the field on the wire (prefix start)
	bodyOff int  // absolute offset of the body
	bodyEnd int  // absolute offset just past the body (== field end)
}

// tagSlot holds one tag-addressed child value in a TLV sub-message.
type tagSlot struct {
	v       Value
	present bool
}

// New returns an empty, mutable message for the given schema, ready for Set.
func New(s *Schema) *Message {
	return &Message{
		schema: s,
		slots:  make([]slot, s.maxDE+1),
		owned:  true,
	}
}

// newDecoded builds a Message that aliases src, populated structurally by the
// engine. sealed controls immutability.
func newDecoded(s *Schema, src []byte, sealed bool) *Message {
	return &Message{
		schema: s,
		src:    src,
		slots:  make([]slot, s.maxDE+1),
		sealed: sealed,
	}
}

// Schema returns the message's schema.
func (m *Message) Schema() *Schema { return m.schema }

// Bitmap returns the derived presence bitmap (data bits only; continuation bits
// are managed by the codec at marshal time).
func (m *Message) Bitmap() Bitmap { return m.bm }

// def returns the FieldDef for a top-level DE (DE 0 is the MTI).
func (m *Message) def(de int) *FieldDef {
	if de == 0 {
		return m.schema.mti
	}
	d, _ := m.schema.Field(de)
	return d
}

// --- presence (no decode) ---

// Has reports presence WITHOUT decoding (a slot/bitmap check only).
func (m *Message) Has(de int) bool {
	if de < 0 || de >= len(m.slots) {
		return false
	}
	return m.slots[de].present
}

// HasP reports presence for a pre-parsed path.
func (m *Message) HasP(p FieldPath) bool {
	if p.Len() == 1 && !p.Head().IsTag() {
		return m.Has(int(p.Head().N))
	}
	_, ok := m.GetP(p)
	return ok
}

// HasS reports presence for a textual path.
func (m *Message) HasS(s string) bool {
	p, err := ParsePath(s)
	if err != nil {
		return false
	}
	return m.HasP(p)
}

// --- read (zero-copy, lazy) ---

// Get returns the zero-copy Value for a top-level DE, decoding lazily on first
// access and memoising the result.
func (m *Message) Get(de int) (Value, bool) {
	if de < 0 || de >= len(m.slots) {
		return Value{}, false
	}
	return m.decodeSlot(de)
}

// GetP returns the Value for a pre-parsed path (the hottest read; no parse).
func (m *Message) GetP(p FieldPath) (Value, bool) {
	if p.Len() == 0 {
		return Value{}, false
	}
	head := p.Head()

	var v Value
	var ok bool
	if m.schema != nil && m.schema.isTLV {
		// Tag-addressed sub-message (BER-TLV): every element is a hex tag.
		v, ok = m.GetTag(tagKey(head))
	} else if head.IsTag() {
		return Value{}, false // positional schema: top-level fields are numeric DEs
	} else {
		v, ok = m.Get(int(head.N))
	}
	if !ok || p.Len() == 1 {
		return v, ok
	}
	// Descend into a composite child.
	sub, ok := v.Composite()
	if !ok {
		return Value{}, false
	}
	return sub.GetP(p.Tail())
}

// GetS returns the Value for a textual path.
func (m *Message) GetS(s string) (Value, bool) {
	p, err := ParsePath(s)
	if err != nil {
		return Value{}, false
	}
	return m.GetP(p)
}

// MTI returns the message type indicator (DE 0).
func (m *Message) MTI() (Value, bool) { return m.Get(0) }

// --- tag-addressed (BER-TLV) children ---

// NewTLV returns an empty tag-addressed child message for a TLV sub-schema.
// Composite codecs (e.g. fieldcodec/tlv) build a child with NewTLV + PutTag
// during decode, and read it back with TagSeq during encode.
func NewTLV(sub *Schema) *Message {
	return &Message{schema: sub, tags: make(map[string]tagSlot), owned: true}
}

// PutTag stores a decoded canonical value under a BER-TLV tag (canonicalised to
// uppercase), preserving first-seen order for deterministic re-encoding.
func (m *Message) PutTag(tag string, v Value) {
	key := strings.ToUpper(tag)
	if m.tags == nil {
		m.tags = make(map[string]tagSlot)
	}
	if _, seen := m.tags[key]; !seen {
		m.tagOrder = append(m.tagOrder, key)
	}
	m.tags[key] = tagSlot{v: v, present: true}
}

// GetTag returns the value stored under a BER-TLV tag.
func (m *Message) GetTag(tag string) (Value, bool) {
	if m.tags == nil {
		return Value{}, false
	}
	ts, ok := m.tags[strings.ToUpper(tag)]
	if !ok || !ts.present {
		return Value{}, false
	}
	return ts.v, true
}

// TagSeq iterates tag-addressed children in first-seen / declaration order.
func (m *Message) TagSeq() iter.Seq2[string, Value] {
	return func(yield func(string, Value) bool) {
		for _, key := range m.tagOrder {
			ts, ok := m.tags[key]
			if !ok || !ts.present {
				continue
			}
			if !yield(key, ts.v) {
				return
			}
		}
	}
}

func (m *Message) decodeSlot(de int) (Value, bool) {
	s := &m.slots[de]
	if !s.present {
		return Value{}, false
	}
	if s.decoded {
		return s.v, true
	}
	def := m.def(de)
	if def == nil {
		return Value{}, false
	}
	if !s.fromSrc {
		// Built field with no src span: the value was set directly.
		s.decoded = true
		return s.v, true
	}
	body := m.src[s.bodyOff:s.bodyEnd]
	var v Value
	if def.Codec != nil {
		dv, err := def.Codec.DecodeBody(body, s.units, def)
		if err != nil {
			return Value{}, false
		}
		v = dv
		v.codec = def.Codec
	} else {
		// Composite/raw: expose the body octets.
		v = Value{raw: body, kind: KindBytes}
	}
	v.off = s.bodyOff
	s.v = v
	s.decoded = true
	return v, true
}

// Fields iterates present data elements (excluding the MTI) in ascending order.
func (m *Message) Fields() iter.Seq2[FieldPath, Value] {
	return func(yield func(FieldPath, Value) bool) {
		for de := 1; de < len(m.slots); de++ {
			if !m.slots[de].present {
				continue
			}
			v, ok := m.decodeSlot(de)
			if !ok {
				continue
			}
			if !yield(DE(de), v) {
				return
			}
		}
	}
}

// --- write (copy-on-write) ---

// Set stores a value at a top-level DE, engaging copy-on-write. It routes the
// value to the field's kind and fails fast with a *FieldError on type mismatch.
func (m *Message) Set(de int, v any) error {
	if m.sealed {
		return &FieldError{Path: DE(de), Offset: -1, Err: errSealed}
	}
	def := m.def(de)
	if def == nil {
		return &FieldError{Path: DE(de), Offset: -1, Err: errUnknownDE}
	}
	val, err := makeValue(def, v)
	if err != nil {
		return err
	}
	val.codec = def.Codec
	if de >= len(m.slots) {
		return &FieldError{Path: DE(de), Offset: -1, Err: errUnknownDE}
	}
	s := &m.slots[de]
	s.v = val
	s.present = true
	s.decoded = true
	s.dirty = true
	s.fromSrc = false
	m.owned = true
	if de >= 1 {
		m.bm.Set(de)
		m.dirty.Set(de)
	}
	return nil
}

// SetP stores a value at a pre-parsed path, creating composite children as
// needed (copy-on-write). It handles top-level DEs, tag-addressed TLV children,
// and one level of composite nesting (e.g. "55.9F26").
func (m *Message) SetP(p FieldPath, v any) error {
	if m.sealed {
		return &FieldError{Path: p, Offset: -1, Err: errSealed}
	}
	if p.Len() == 0 {
		return &FieldError{Path: p, Offset: -1, Err: errNotPresent}
	}
	head := p.Head()

	// Inside a tag-addressed (TLV) sub-message: set a tag value.
	if m.schema != nil && m.schema.isTLV {
		tag := tagKey(head)
		tagDef, _ := m.schema.LookupTag(tag)
		if p.Len() > 1 {
			return &FieldError{Path: p, Offset: -1, Err: fmt.Errorf("nested constructed TLV write %q not supported", p.String())}
		}
		val, err := makeValue(tagDef, v)
		if err != nil {
			return err
		}
		if tagDef != nil {
			val.codec = tagDef.Codec
		}
		m.PutTag(tag, val)
		m.owned = true
		return nil
	}

	if head.IsTag() {
		return &FieldError{Path: p, Offset: -1, Err: errUnknownDE}
	}
	de := int(head.N)
	if p.Len() == 1 {
		return m.Set(de, v)
	}

	// Descend into a composite child.
	def := m.def(de)
	if def == nil || def.Sub == nil {
		return &FieldError{Path: p, Offset: -1, Err: fmt.Errorf("DE %d is not a composite", de)}
	}
	child, err := m.ensureChild(de, def)
	if err != nil {
		return err
	}
	return child.SetP(p.Tail(), v)
}

// ensureChild returns the mutable composite child for a top-level composite DE,
// decoding an existing one from src or creating a fresh child, and marking the
// parent dirty so the child re-encodes on Marshal.
func (m *Message) ensureChild(de int, def *FieldDef) (*Message, error) {
	if de >= len(m.slots) {
		return nil, &FieldError{Path: DE(de), Offset: -1, Err: errUnknownDE}
	}
	s := &m.slots[de]
	if s.present {
		v, ok := m.decodeSlot(de)
		if ok {
			if child, ok := v.Composite(); ok {
				s.dirty = true
				s.fromSrc = false
				m.owned = true
				return child, nil
			}
		}
	}
	var child *Message
	if def.Sub != nil && def.Sub.isTLV {
		child = NewTLV(def.Sub)
	} else {
		child = New(def.Sub)
	}
	s.v = CompositeValue(nil, child)
	s.present = true
	s.decoded = true
	s.dirty = true
	s.fromSrc = false
	m.owned = true
	if de >= 1 {
		m.bm.Set(de)
		m.dirty.Set(de)
	}
	return child, nil
}

// SetS stores a value at a textual path.
func (m *Message) SetS(s string, v any) error {
	p, err := ParsePath(s)
	if err != nil {
		return err
	}
	return m.SetP(p, v)
}

// Unset removes a top-level DE (no-op on a sealed message).
func (m *Message) Unset(de int) {
	if m.sealed || de < 0 || de >= len(m.slots) {
		return
	}
	s := &m.slots[de]
	if !s.present {
		return
	}
	*s = slot{}
	if de >= 1 {
		m.bm.Clear(de)
	}
	m.owned = true
}

// UnsetP removes the field at a pre-parsed path.
func (m *Message) UnsetP(p FieldPath) {
	if p.Len() == 1 && !p.Head().IsTag() {
		m.Unset(int(p.Head().N))
	}
}

// UnsetS removes the field at a textual path.
func (m *Message) UnsetS(s string) {
	if p, err := ParsePath(s); err == nil {
		m.UnsetP(p)
	}
}

// --- lifecycle ---

// Seal returns an immutable view of the message. Mutating a sealed message
// fails; Clone first to obtain a mutable copy that shares src (structural
// sharing — only touched fields allocate).
func (m *Message) Seal() *Message {
	m.sealed = true
	return m
}

// Clone returns a mutable copy that shares src with the original. No field bytes
// are copied until a field is Set (copy-on-write).
func (m *Message) Clone() *Message {
	c := &Message{
		schema: m.schema,
		src:    m.src,
		bm:     m.bm,
		dirty:  m.dirty,
		slots:  make([]slot, len(m.slots)),
	}
	copy(c.slots, m.slots)
	return c
}

// makeValue builds a canonical-form Value for a Set, routed by the field kind
// and the Go type supplied.
func makeValue(def *FieldDef, in any) (Value, error) {
	kind := KindString
	if def != nil && def.Kind != KindInvalid {
		kind = def.Kind
	}
	switch x := in.(type) {
	case Value:
		return x, nil
	case string:
		return scalarFromBytes(kind, []byte(x))
	case []byte:
		b := append([]byte(nil), x...)
		k := kind
		if def == nil || def.Kind == KindInvalid {
			k = KindBytes
		}
		return Value{raw: b, kind: k}, nil
	case int:
		return scalarFromInt(kind, int64(x))
	case int64:
		return scalarFromInt(kind, x)
	case uint64:
		return Value{raw: []byte(strconv.FormatUint(x, 10)), kind: numericKind(kind)}, nil
	case Decimal:
		return Value{dec: x, raw: []byte(x.String()), kind: KindAmount}, nil
	case Amount:
		return Value{dec: x.Decimal, raw: []byte(x.Decimal.String()), kind: KindAmount}, nil
	default:
		return Value{}, &FieldError{Path: defPath(def), Offset: -1, Err: fmt.Errorf("unsupported value type %T", in)}
	}
}

func scalarFromBytes(kind Kind, b []byte) (Value, error) {
	switch kind {
	case KindAmount:
		n, err := strconv.ParseInt(string(b), 10, 64)
		if err != nil {
			return Value{}, &FieldError{Offset: -1, Err: err}
		}
		return Value{dec: Decimal{Unscaled: n}, raw: b, kind: KindAmount}, nil
	case KindBytes:
		return Value{raw: b, kind: KindBytes}, nil
	case KindNumeric:
		return Value{raw: b, kind: KindNumeric}, nil
	default:
		return Value{raw: b, kind: KindString}, nil
	}
}

func scalarFromInt(kind Kind, n int64) (Value, error) {
	if kind == KindAmount {
		return Value{dec: Decimal{Unscaled: n}, raw: []byte(strconv.FormatInt(n, 10)), kind: KindAmount}, nil
	}
	return Value{raw: []byte(strconv.FormatInt(n, 10)), kind: numericKind(kind)}, nil
}

// numericKind keeps an explicit numeric/bytes kind but promotes a default string
// field to numeric when an integer is stored.
func numericKind(kind Kind) Kind {
	if kind == KindString || kind == KindInvalid {
		return KindNumeric
	}
	return kind
}

func defPath(def *FieldDef) FieldPath {
	if def == nil {
		return FieldPath{}
	}
	return def.Path
}
