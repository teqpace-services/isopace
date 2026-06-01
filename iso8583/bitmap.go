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
	"math/bits"
	"strconv"
	"strings"
)

// Bitmap is a 192-bit presence set (primary, secondary, tertiary). Field n in
// 1..192 maps to words[(n-1)/64], bit (n-1)%64 counted from the MSB — the
// big-endian bit order used on the wire.
//
// Bits 1 and 65 are continuation bits managed entirely by the engine: bit 1
// signals a secondary bitmap, bit 65 a tertiary one. Application code never
// sets bitmap bits; the wire bitmap is always derived from the set of present
// fields at marshal time, so bitmap/content desync is structurally impossible.
type Bitmap struct {
	words [3]uint64
}

// maxDEIndex is the highest addressable data element.
const maxDEIndex = 192

// continuation bit positions (1-based DE numbers) reserved for the engine.
const (
	contSecondary = 1  // presence of the secondary bitmap (DEs 65..128)
	contTertiary  = 65 // presence of the tertiary bitmap (DEs 129..192)
)

func wordBit(de int) (word int, mask uint64) {
	idx := de - 1
	return idx / 64, uint64(1) << uint(63-idx%64)
}

// IsSet reports whether data element de (1..192) is present.
func (b Bitmap) IsSet(de int) bool {
	if de < 1 || de > maxDEIndex {
		return false
	}
	w, mask := wordBit(de)
	return b.words[w]&mask != 0
}

// Set marks de present. Intended for engine use; application code never calls
// it directly (presence is derived from set fields at marshal time).
func (b *Bitmap) Set(de int) {
	if de < 1 || de > maxDEIndex {
		return
	}
	w, mask := wordBit(de)
	b.words[w] |= mask
}

// Clear marks de absent.
func (b *Bitmap) Clear(de int) {
	if de < 1 || de > maxDEIndex {
		return
	}
	w, mask := wordBit(de)
	b.words[w] &^= mask
}

// Word returns the i-th 64-bit word (0=primary, 1=secondary, 2=tertiary) for
// codecs that emit the raw wire form.
func (b Bitmap) Word(i int) uint64 { return b.words[i] }

// SetWord installs the i-th 64-bit word; for codecs reconstructing a Bitmap
// from wire bytes.
func (b *Bitmap) SetWord(i int, v uint64) { b.words[i] = v }

// Count returns the number of present data elements, excluding the
// continuation bits.
func (b Bitmap) Count() int {
	n := bits.OnesCount64(b.words[0]) + bits.OnesCount64(b.words[1]) + bits.OnesCount64(b.words[2])
	if b.IsSet(contSecondary) {
		n--
	}
	if b.IsSet(contTertiary) {
		n--
	}
	return n
}

// Width returns the wire width in bits implied by the continuation bits:
// 64 (primary only), 128 (+secondary), or 192 (+tertiary).
func (b Bitmap) Width() int {
	switch {
	case b.IsSet(contTertiary):
		return 192
	case b.IsSet(contSecondary):
		return 128
	default:
		return 64
	}
}

// Range yields the present data elements in ascending order, excluding the
// continuation bits. It is a range-over-func iterator:
//
//	for de := range b.Range { ... }
func (b Bitmap) Range(yield func(de int) bool) {
	for w := 0; w < 3; w++ {
		word := b.words[w]
		base := w * 64
		for word != 0 {
			lz := bits.LeadingZeros64(word)
			de := base + lz + 1
			if de != contSecondary && de != contTertiary {
				if !yield(de) {
					return
				}
			}
			word &^= uint64(1) << uint(63-lz)
		}
	}
}

// String renders a human-readable dump of present DEs for logs, e.g.
// "Bitmap{2 3 4 11 41} width=64".
func (b Bitmap) String() string {
	var sb strings.Builder
	sb.WriteString("Bitmap{")
	first := true
	b.Range(func(de int) bool {
		if !first {
			sb.WriteByte(' ')
		}
		first = false
		sb.WriteString(strconv.Itoa(de))
		return true
	})
	sb.WriteString("} width=")
	sb.WriteString(strconv.Itoa(b.Width()))
	return sb.String()
}
