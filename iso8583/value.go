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

package iso8583

import "strconv"

// Kind classifies a Value's interpretation.
type Kind uint8

const (
	KindInvalid   Kind = iota // zero value; not present / undecoded
	KindString                // text (charset applied at decode time)
	KindNumeric               // digit string (ASCII digits in canonical form)
	KindBytes                 // raw octets
	KindAmount                // signed scaled money -> Decimal
	KindComposite             // has children (subfields / BER-TLV)
)

// String renders the Kind name for diagnostics.
func (k Kind) String() string {
	switch k {
	case KindString:
		return "string"
	case KindNumeric:
		return "numeric"
	case KindBytes:
		return "bytes"
	case KindAmount:
		return "amount"
	case KindComposite:
		return "composite"
	default:
		return "invalid"
	}
}

// Value is a zero-copy view into the owning Message's source buffer plus a tag
// describing interpretation. Copying a Value is cheap (it is a small header);
// it never owns or mutates the underlying bytes in the read path. See the
// canonical value form documented in codec_iface.go.
type Value struct {
	raw   []byte     // canonical bytes; aliases Message.src for ASCII/binary, owned otherwise
	kind  Kind       // interpretation
	codec FieldCodec // how the field was/should be (de)coded; may be nil for built scalars
	off   int        // absolute byte offset in src for error reporting, or -1
	sub   *Message   // non-nil for KindComposite (lazy child message)
	dec   Decimal    // decoded amount for KindAmount; zero otherwise
}

// Kind returns the value's interpretation.
func (v Value) Kind() Kind { return v.kind }

// IsZero reports whether the value is the zero/absent value.
func (v Value) IsZero() bool { return v.kind == KindInvalid }

// Bytes returns the raw canonical view with ZERO allocation and ZERO copy. The
// returned slice ALIASES the source buffer and MUST NOT be mutated. This is the
// documented zero-copy primitive: hot-path routing should compare against
// Bytes() with bytes.Equal / bytes.HasPrefix, never String().
func (v Value) Bytes() []byte { return v.raw }

// BytesCopy returns an owned copy, safe to retain and mutate.
func (v Value) BytesCopy() []byte {
	if v.raw == nil {
		return nil
	}
	c := make([]byte, len(v.raw))
	copy(c, v.raw)
	return c
}

// String decodes the value as text. Because Go strings cannot alias a []byte
// without a copy, this is the single unavoidable allocation for text. Prefer
// Bytes() on the hot path.
func (v Value) String() (string, error) {
	switch v.kind {
	case KindString, KindNumeric:
		return string(v.raw), nil
	case KindBytes:
		return string(v.raw), nil
	case KindAmount:
		return v.dec.String(), nil
	default:
		return "", v.fieldErr(errNotText)
	}
}

// Int parses the value's digits as a signed integer without an intermediate
// string, reporting a *FieldError on malformed input.
func (v Value) Int() (int64, error) {
	switch v.kind {
	case KindNumeric, KindString:
		n, err := parseInt(v.raw)
		if err != nil {
			return 0, v.fieldErr(err)
		}
		return n, nil
	case KindAmount:
		return v.dec.Unscaled, nil
	default:
		return 0, v.fieldErr(errNotNumeric)
	}
}

// Uint parses the value's digits as an unsigned integer.
func (v Value) Uint() (uint64, error) {
	switch v.kind {
	case KindNumeric, KindString:
		u, err := parseUint(v.raw)
		if err != nil {
			return 0, v.fieldErr(err)
		}
		return u, nil
	default:
		return 0, v.fieldErr(errNotNumeric)
	}
}

// Decimal returns the exact fixed-point amount (never float64).
func (v Value) Decimal() (Decimal, error) {
	switch v.kind {
	case KindAmount:
		return v.dec, nil
	case KindNumeric:
		n, err := parseInt(v.raw)
		if err != nil {
			return Decimal{}, v.fieldErr(err)
		}
		return Decimal{Unscaled: n, Scale: 0}, nil
	default:
		return Decimal{}, v.fieldErr(errWrongKind)
	}
}

// Composite returns the lazily-decoded child message for KindComposite fields
// (subfield groups or BER-TLV), enabling nested addressing.
func (v Value) Composite() (*Message, bool) {
	if v.kind == KindComposite && v.sub != nil {
		return v.sub, true
	}
	return nil, false
}

func (v Value) fieldErr(cause error) *FieldError {
	return &FieldError{Offset: v.off, Err: cause}
}

// parseInt parses ASCII decimal digits, allowing a single leading sign.
func parseInt(b []byte) (int64, error) {
	if len(b) == 0 {
		return 0, errNotNumeric
	}
	return strconv.ParseInt(btoaTrim(b), 10, 64)
}

func parseUint(b []byte) (uint64, error) {
	if len(b) == 0 {
		return 0, errNotNumeric
	}
	return strconv.ParseUint(btoaTrim(b), 10, 64)
}

// btoaTrim converts a digit slice to a string for strconv, preserving content.
// (A dedicated digit scanner is a Phase-5 optimisation; correctness first.)
func btoaTrim(b []byte) string { return string(b) }

// Value constructors for codec packages.
//
// Value's fields are unexported, so FieldCodec implementations living in other
// packages (fieldcodec/, fieldcodec/tlv/, …) build canonical Values through
// these constructors. They must honour the canonical value form documented in
// codec_iface.go: text/numeric raw is ASCII/UTF-8, bytes raw is the octets,
// amount carries the exact Decimal.

// StringValue builds a canonical KindString value from decoded text bytes.
func StringValue(raw []byte) Value { return Value{raw: raw, kind: KindString, off: -1} }

// NumericValue builds a canonical KindNumeric value from ASCII decimal digits.
func NumericValue(raw []byte) Value { return Value{raw: raw, kind: KindNumeric, off: -1} }

// BytesValue builds a canonical KindBytes value aliasing the given octets.
func BytesValue(raw []byte) Value { return Value{raw: raw, kind: KindBytes, off: -1} }

// AmountValue builds a canonical KindAmount value from the exact Decimal. raw
// may be nil or the wire-ish digits for Bytes()/String() display.
func AmountValue(dec Decimal, raw []byte) Value {
	return Value{raw: raw, kind: KindAmount, dec: dec, off: -1}
}

// CompositeValue builds a KindComposite value wrapping a decoded child message.
func CompositeValue(raw []byte, sub *Message) Value {
	return Value{raw: raw, kind: KindComposite, sub: sub, off: -1}
}
