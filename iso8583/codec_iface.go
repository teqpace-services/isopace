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

// This file declares the codec contracts. Per ARCHITECTURE.md §10 C1 the
// interfaces live in package iso8583 (the model), and the fieldcodec/ and
// lengthcodec/ packages import iso8583 and provide the concrete
// implementations plus a Registry. This breaks the otherwise-cyclic import
// (the model references the codec interface; codecs reference Value/FieldDef)
// and keeps the dependency graph acyclic: iso8583 imports neither codec
// package, and SchemaBuilder.Build takes no registry — codecs are passed in
// already resolved.
//
// Canonical value form (the contract between codecs and Value getters).
// Because a single FieldCodec interface cannot carry per-type conversions for
// every Value getter, DecodeBody must return a Value in canonical form, and
// EncodeBody must accept the same canonical form:
//
//   - KindString  : Value.raw holds decoded text bytes (a charset codec such as
//                   EBCDIC transcodes to UTF-8/ASCII here; this is the single
//                   allowed allocation). Value.String() returns string(raw).
//   - KindNumeric : Value.raw holds ASCII decimal digits. Value.Int()/Uint()
//                   parse them directly.
//   - KindBytes   : Value.raw aliases the body octets (true zero copy).
//                   Value.Bytes() returns them.
//   - KindAmount  : Value.dec holds the exact decoded Decimal; Value.raw may
//                   alias the wire bytes for Bytes(). Value.Decimal() returns dec.
//   - KindComposite: Value.sub holds the lazily-decoded child *Message.
//
// A codec whose representation aliases the source buffer (ASCII, binary) MUST
// return sub-slices of body for zero copy; only transcoding or digit parsing
// may allocate, and only lazily.

// FieldCodec encodes/decodes one field's BODY. The length prefix is handled
// separately by a LengthCodec, so value and length codecs compose orthogonally.
type FieldCodec interface {
	// DecodeBody interprets body (already length-delimited by the engine) into
	// a canonical-form Value (see the contract above).
	DecodeBody(body []byte, def *FieldDef) (Value, error)

	// EncodeBody appends the wire body of a canonical-form v to dst and returns
	// the grown slice (append style; reuses dst capacity).
	EncodeBody(dst []byte, v Value, def *FieldDef) ([]byte, error)

	// Kind is the natural model kind this codec yields (drives binding and
	// rendering).
	Kind() Kind

	// Name is the registry key, e.g. "char.ascii", "num.bcd", "tlv.ber".
	Name() string
}

// SpanCodec is an OPTIONAL extension implemented by codecs that can locate a
// field body's span without decoding it. Unmarshal prefers Span for cheap
// structural parsing; codecs that do not implement it fall back to DecodeBody.
//
// Span receives the bytes from the start of the body to the end of the source
// buffer and returns the body's [start,end) relative to that slice.
type SpanCodec interface {
	FieldCodec
	Span(body []byte, def *FieldDef) (start, end int, err error)
}

// LengthCodec handles the length prefix (fixed / LL / LLL / LLLL) in a chosen
// representation (ASCII / BCD / Binary / EBCDIC). A nil LengthCodec on a
// FieldDef means a fixed-width field of FieldDef.MaxLen wire bytes with no
// prefix; the engine handles that case directly so MTI and fixed fields need
// no length codec.
//
// Lengths are expressed in WIRE BODY BYTES. A packed (BCD) codec that reads a
// digit count from the prefix converts to body bytes internally using def
// (which carries the value codec), keeping the two axes orthogonal.
type LengthCodec interface {
	// ReadLen consumes the length prefix from src at off, returning the number
	// of body bytes that follow and the offset just past the prefix.
	ReadLen(src []byte, off int, def *FieldDef) (bodyLen, next int, err error)
	// WriteLen appends a length prefix for a body of n wire bytes.
	WriteLen(dst []byte, n int, def *FieldDef) ([]byte, error)
	Name() string
}

// BitmapCodec encodes/decodes the message bitmap in a chosen wire
// representation (binary, ASCII-hex, EBCDIC-hex). The continuation bits and the
// number of emitted levels are managed by the codec from the present-field set.
type BitmapCodec interface {
	// ReadBitmap decodes the wire bitmap starting at src[off], honouring up to
	// maxLevels (1=primary, 2=+secondary, 3=+tertiary) via the continuation
	// bits, and returns the Bitmap and the offset just past it.
	ReadBitmap(src []byte, off, maxLevels int) (Bitmap, int, error)
	// WriteBitmap appends the wire bitmap derived from b, emitting only as many
	// levels as b's set bits require (bounded by maxLevels) and setting the
	// continuation bits accordingly.
	WriteBitmap(dst []byte, b Bitmap, maxLevels int) ([]byte, error)
	Name() string
}
