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
	"encoding/binary"
	"encoding/hex"

	"github.com/teqpace-services/isopace/internal/ebcdic"
	"github.com/teqpace-services/isopace/iso8583"
)

// Bitmap codecs render the message bitmap as binary, ASCII-hex or EBCDIC-hex.
// Each 64-bit word is one "level"; the continuation bit (the MSB of a word)
// signals that the next level follows. The number of levels emitted is derived
// from the present-field set, bounded by maxLevels.

const contMSB = uint64(1) << 63 // continuation bit: MSB of a word

// levelWords returns the words to emit (with continuation bits set) and how many
// levels (1..3) are required for the present-field set, bounded by maxLevels.
func levelWords(bm iso8583.Bitmap, maxLevels int) (w [3]uint64, n int) {
	w[0], w[1], w[2] = bm.Word(0), bm.Word(1), bm.Word(2)
	need3 := w[2] != 0 && maxLevels >= 3
	need2 := (w[1] != 0 || need3) && maxLevels >= 2
	n = 1
	if need2 {
		n = 2
		w[0] |= contMSB
	}
	if need3 {
		n = 3
		w[1] |= contMSB
	}
	return w, n
}

// wordRepr serialises/parses a single 64-bit bitmap word.
type wordRepr interface {
	wireLen() int                    // bytes one word occupies on the wire
	put(dst []byte, w uint64) []byte // append one word
	get(src []byte) (uint64, bool)   // parse one word from the front of src
	name() string
}

type bitmapCodec struct{ repr wordRepr }

func (c bitmapCodec) Name() string { return c.repr.name() }

func (c bitmapCodec) ReadBitmap(src []byte, off, maxLevels int) (iso8583.Bitmap, int, error) {
	var bm iso8583.Bitmap
	wl := c.repr.wireLen()
	for level := 0; level < 3; level++ {
		// Overflow-safe bounds check (off can be large/adversarial): never form
		// off+wl, which could wrap negative and pass a naive comparison.
		if off < 0 || off > len(src) || len(src)-off < wl {
			return bm, 0, ErrShortBitmap
		}
		w, ok := c.repr.get(src[off : off+wl])
		if !ok {
			return bm, 0, ErrBadHex
		}
		bm.SetWord(level, w)
		off += wl
		if w&contMSB == 0 || level+1 >= maxLevels {
			break
		}
	}
	return bm, off, nil
}

func (c bitmapCodec) WriteBitmap(dst []byte, bm iso8583.Bitmap, maxLevels int) ([]byte, error) {
	w, n := levelWords(bm, maxLevels)
	for i := 0; i < n; i++ {
		dst = c.repr.put(dst, w[i])
	}
	return dst, nil
}

// binary word: 8 raw bytes, big-endian.
type binWord struct{}

func (binWord) wireLen() int { return 8 }
func (binWord) put(dst []byte, w uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], w)
	return append(dst, b[:]...)
}
func (binWord) get(src []byte) (uint64, bool) { return binary.BigEndian.Uint64(src), true }
func (binWord) name() string                  { return "bitmap.bin" }

// hex word: 16 ASCII hex chars (uppercase on output).
type hexWord struct{}

func (hexWord) wireLen() int { return 16 }
func (hexWord) put(dst []byte, w uint64) []byte {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], w)
	var enc [16]byte
	hex.Encode(enc[:], raw[:])
	return append(dst, upperHex(enc[:])...)
}
func (hexWord) get(src []byte) (uint64, bool) {
	var raw [8]byte
	if _, err := hex.Decode(raw[:], src); err != nil {
		return 0, false
	}
	return binary.BigEndian.Uint64(raw[:]), true
}
func (hexWord) name() string { return "bitmap.hex" }

// ebcdicWord: 16 EBCDIC-hex chars (CP037).
type ebcdicWord struct{}

func (ebcdicWord) wireLen() int { return 16 }
func (ebcdicWord) put(dst []byte, w uint64) []byte {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], w)
	var enc [16]byte
	hex.Encode(enc[:], raw[:])
	return append(dst, ebcdic.CP037.Encode(upperHex(enc[:]))...)
}
func (ebcdicWord) get(src []byte) (uint64, bool) {
	ascii := ebcdic.CP037.Decode(src)
	var raw [8]byte
	if _, err := hex.Decode(raw[:], ascii); err != nil {
		return 0, false
	}
	return binary.BigEndian.Uint64(raw[:]), true
}
func (ebcdicWord) name() string { return "bitmap.ebcdic" }

func upperHex(b []byte) []byte {
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			b[i] = c - 'a' + 'A'
		}
	}
	return b
}

// Catalog singletons.
var (
	BitmapBinary iso8583.BitmapCodec = bitmapCodec{repr: binWord{}}
	BitmapHex    iso8583.BitmapCodec = bitmapCodec{repr: hexWord{}}
	BitmapEBCDIC iso8583.BitmapCodec = bitmapCodec{repr: ebcdicWord{}}
)
