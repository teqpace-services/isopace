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

package fieldcodec

import (
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/lengthcodec"
)

// Registry resolves codecs by name for schema assembly (packager/, generic YAML
// loader). It is populated explicitly by DefaultRegistry — never by init() side
// effects — so global state stays visible and import order never matters.
type Registry struct {
	values  map[string]iso8583.FieldCodec
	lengths map[string]iso8583.LengthCodec
	bitmaps map[string]iso8583.BitmapCodec
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		values:  make(map[string]iso8583.FieldCodec),
		lengths: make(map[string]iso8583.LengthCodec),
		bitmaps: make(map[string]iso8583.BitmapCodec),
	}
}

// Register adds (or replaces) a value codec under its Name().
func (r *Registry) Register(c iso8583.FieldCodec) { r.values[c.Name()] = c }

// RegisterLength adds (or replaces) a length codec under its Name().
func (r *Registry) RegisterLength(l iso8583.LengthCodec) { r.lengths[l.Name()] = l }

// RegisterBitmap adds (or replaces) a bitmap codec under its Name().
func (r *Registry) RegisterBitmap(b iso8583.BitmapCodec) { r.bitmaps[b.Name()] = b }

// Lookup resolves a value codec by name.
func (r *Registry) Lookup(name string) (iso8583.FieldCodec, bool) {
	c, ok := r.values[name]
	return c, ok
}

// LookupLength resolves a length codec by name.
func (r *Registry) LookupLength(name string) (iso8583.LengthCodec, bool) {
	l, ok := r.lengths[name]
	return l, ok
}

// LookupBitmap resolves a bitmap codec by name.
func (r *Registry) LookupBitmap(name string) (iso8583.BitmapCodec, bool) {
	b, ok := r.bitmaps[name]
	return b, ok
}

// DefaultRegistry returns a freshly-built registry pre-populated with the full
// catalog: every value codec, every length codec, and every bitmap codec.
// Callers may add custom codecs to the returned instance.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, c := range []iso8583.FieldCodec{
		ASCII, EBCDIC037, EBCDIC1047, UTF8, BINARY,
		NumASCII, NumEBCDIC, BCD, RBCD, NumBinary,
		AmountASCII, AmountBCD, AmountBinary,
	} {
		r.Register(c)
	}
	for _, l := range lengthcodec.All() {
		r.RegisterLength(l)
	}
	for _, b := range []iso8583.BitmapCodec{BitmapBinary, BitmapHex, BitmapEBCDIC} {
		r.RegisterBitmap(b)
	}
	return r
}
