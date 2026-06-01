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

// Package ebcdic provides EBCDIC<->ASCII byte translation for the code pages
// used by ISO-8583 EBCDIC links: CP037 (US/Canada) and CP1047 (Open Systems /
// Latin-1). The tables cover the full invariant subset ISO-8583 actually
// carries — decimal digits, A–Z, a–z, space and the standard punctuation — plus
// the documented per-page differences (notably ^ [ ]). Code points outside the
// covered set decode to '?' and unmappable ASCII encodes to EBCDIC 0x6F ('?');
// this is a financial field transcoder, not a general-purpose Unicode codec.
package ebcdic

const (
	sub      = 0x3F // ASCII '?' placeholder for un-decodable EBCDIC bytes
	ebcdicQ  = 0x6F // EBCDIC '?' placeholder for un-encodable ASCII bytes
	ebcdicSP = 0x40 // EBCDIC space
)

// Table is a bidirectional EBCDIC<->ASCII translation.
type Table struct {
	e2a [256]byte // EBCDIC byte -> ASCII byte
	a2e [256]byte // ASCII byte  -> EBCDIC byte
}

// DecodeByte translates one EBCDIC byte to ASCII.
func (t *Table) DecodeByte(b byte) byte { return t.e2a[b] }

// EncodeByte translates one ASCII byte to EBCDIC.
func (t *Table) EncodeByte(b byte) byte { return t.a2e[b] }

// Decode translates an EBCDIC slice to a freshly-allocated ASCII slice.
func (t *Table) Decode(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = t.e2a[b[i]]
	}
	return out
}

// Encode translates an ASCII slice to a freshly-allocated EBCDIC slice.
func (t *Table) Encode(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = t.a2e[b[i]]
	}
	return out
}

type pair struct {
	e byte // EBCDIC code
	a byte // ASCII code
}

// invariant lists the EBCDIC<->ASCII assignments shared by CP037 and CP1047 for
// the characters ISO-8583 uses. Digits and letters are filled by ranges below.
var invariant = []pair{
	{0x40, ' '},
	{0x4B, '.'}, {0x4C, '<'}, {0x4D, '('}, {0x4E, '+'}, {0x4F, '|'},
	{0x50, '&'},
	{0x5A, '!'}, {0x5B, '$'}, {0x5C, '*'}, {0x5D, ')'}, {0x5E, ';'},
	{0x60, '-'}, {0x61, '/'},
	{0x6B, ','}, {0x6C, '%'}, {0x6D, '_'}, {0x6E, '>'}, {0x6F, '?'},
	{0x79, '`'},
	{0x7A, ':'}, {0x7B, '#'}, {0x7C, '@'}, {0x7D, '\''}, {0x7E, '='}, {0x7F, '"'},
	{0xA1, '~'},
	{0xC0, '{'}, {0xD0, '}'}, {0xE0, '\\'},
}

// rangeMap is a contiguous EBCDIC..ASCII range, e.g. 0xF0.. -> '0'..
type rangeMap struct {
	eStart byte
	aStart byte
	n      int
}

var ranges = []rangeMap{
	{0xF0, '0', 10}, // digits
	{0xC1, 'A', 9},  // A-I
	{0xD1, 'J', 9},  // J-R
	{0xE2, 'S', 8},  // S-Z
	{0x81, 'a', 9},  // a-i
	{0x91, 'j', 9},  // j-r
	{0xA2, 's', 8},  // s-z
}

// CP037 (US/Canada) and CP1047 (Open Systems) differ for a few symbols.
var (
	cp037Extra  = []pair{{0xBA, '['}, {0xBB, ']'}} // '^' has no ASCII slot in CP037
	cp1047Extra = []pair{{0x5F, '^'}, {0xAD, '['}, {0xBD, ']'}}
)

// CP037 is the IBM-037 EBCDIC code page (US/Canada).
var CP037 = newTable(cp037Extra)

// CP1047 is the IBM-1047 EBCDIC code page (Open Systems / Latin-1).
var CP1047 = newTable(cp1047Extra)

func newTable(extra []pair) *Table {
	t := &Table{}
	for i := range t.e2a {
		t.e2a[i] = sub
	}
	for i := range t.a2e {
		t.a2e[i] = ebcdicQ
	}
	set := func(e, a byte) {
		t.e2a[e] = a
		t.a2e[a] = e
	}
	for _, p := range invariant {
		set(p.e, p.a)
	}
	for _, r := range ranges {
		for i := 0; i < r.n; i++ {
			set(r.eStart+byte(i), r.aStart+byte(i))
		}
	}
	for _, p := range extra {
		set(p.e, p.a)
	}
	// Space must always round-trip even though '?' is the encode default.
	t.a2e[' '] = ebcdicSP
	return t
}
