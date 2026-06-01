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

// Package lengthcodec implements the orthogonal length-prefix axis of the
// Isopace wire engine: Fixed plus LL..LLLLLL variable prefixes in ASCII, BCD,
// binary and EBCDIC representations. A field's wire behaviour is the pair
// (LengthCodec, fieldcodec.FieldCodec), so these compose freely with any value
// codec instead of jPOS's combinatorial IF* class zoo.
//
// Per the iso8583 contract a length prefix carries a LOGICAL UNIT COUNT
// (digits/characters/octets); the engine converts units to a wire byte span via
// the value codec's optional WidthCodec.
package lengthcodec

import (
	"errors"
	"strconv"

	"github.com/teqpace-services/isopace/internal/bcd"
	"github.com/teqpace-services/isopace/internal/ebcdic"
	"github.com/teqpace-services/isopace/iso8583"
)

// Errors returned by the length codecs.
var (
	ErrTruncated = errors.New("lengthcodec: truncated length prefix")
	ErrOverflow  = errors.New("lengthcodec: length exceeds prefix capacity")
	ErrBadDigit  = errors.New("lengthcodec: non-digit in length prefix")
)

// fixed is a no-prefix codec; the body width comes from FieldDef.MaxLen.
type fixed struct{}

func (fixed) ReadLen(_ []byte, off int, def *iso8583.FieldDef) (units, next int, err error) {
	return def.MaxLen, off, nil
}
func (fixed) WriteLen(dst []byte, _ int, _ *iso8583.FieldDef) ([]byte, error) { return dst, nil }
func (fixed) Name() string                                                    { return "len.fixed" }

// repr selects how a variable prefix encodes its decimal count.
type repr uint8

const (
	reprASCII  repr = iota // n ASCII digit bytes
	reprEBCDIC             // n EBCDIC digit bytes (CP037; digits are page-invariant)
	reprBCD                // n digits packed two-per-byte
	reprBinary             // n raw bytes, big-endian unsigned
)

// varLen is a variable-length prefix of a fixed number of decimal digits (for
// ASCII/EBCDIC/BCD) or bytes (for binary).
type varLen struct {
	name   string
	digits int // decimal digits in the prefix (ASCII/EBCDIC/BCD)
	bytes  int // raw bytes in the prefix (binary)
	repr   repr
}

func (v varLen) Name() string { return v.name }

func (v varLen) ReadLen(src []byte, off int, _ *iso8583.FieldDef) (units, next int, err error) {
	switch v.repr {
	case reprASCII:
		if off+v.digits > len(src) {
			return 0, 0, ErrTruncated
		}
		n, err := atoiDigits(src[off : off+v.digits])
		if err != nil {
			return 0, 0, err
		}
		return n, off + v.digits, nil
	case reprEBCDIC:
		if off+v.digits > len(src) {
			return 0, 0, ErrTruncated
		}
		n, err := atoiDigits(ebcdic.CP037.Decode(src[off : off+v.digits]))
		if err != nil {
			return 0, 0, err
		}
		return n, off + v.digits, nil
	case reprBCD:
		nb := bcd.PackedLen(v.digits)
		if off+nb > len(src) {
			return 0, 0, ErrTruncated
		}
		digits, err := bcd.Unpack(src[off:off+nb], v.digits)
		if err != nil {
			return 0, 0, err
		}
		n, err := atoiDigits(digits)
		if err != nil {
			return 0, 0, err
		}
		return n, off + nb, nil
	case reprBinary:
		if off+v.bytes > len(src) {
			return 0, 0, ErrTruncated
		}
		n := 0
		for i := 0; i < v.bytes; i++ {
			n = n<<8 | int(src[off+i])
		}
		return n, off + v.bytes, nil
	default:
		return 0, 0, ErrTruncated
	}
}

func (v varLen) WriteLen(dst []byte, units int, _ *iso8583.FieldDef) ([]byte, error) {
	if units < 0 {
		return nil, ErrOverflow
	}
	switch v.repr {
	case reprASCII:
		s, err := decimalPad(units, v.digits)
		if err != nil {
			return nil, err
		}
		return append(dst, s...), nil
	case reprEBCDIC:
		s, err := decimalPad(units, v.digits)
		if err != nil {
			return nil, err
		}
		return append(dst, ebcdic.CP037.Encode(s)...), nil
	case reprBCD:
		s, err := decimalPad(units, v.digits)
		if err != nil {
			return nil, err
		}
		packed, err := bcd.Pack(s)
		if err != nil {
			return nil, err
		}
		return append(dst, packed...), nil
	case reprBinary:
		if units >= 1<<(8*v.bytes) {
			return nil, ErrOverflow
		}
		for i := v.bytes - 1; i >= 0; i-- {
			dst = append(dst, byte(units>>(8*i)))
		}
		return dst, nil
	default:
		return nil, ErrOverflow
	}
}

// decimalPad renders n as exactly width zero-padded ASCII digits, erroring if it
// does not fit.
func decimalPad(n, width int) ([]byte, error) {
	s := strconv.Itoa(n)
	if len(s) > width {
		return nil, ErrOverflow
	}
	out := make([]byte, width)
	gap := width - len(s)
	for i := 0; i < gap; i++ {
		out[i] = '0'
	}
	copy(out[gap:], s)
	return out, nil
}

func atoiDigits(b []byte) (int, error) {
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, ErrBadDigit
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// The catalog: composable length prefixes referenced by name from schemas, YAML,
// or directly. Each is a stateless singleton.
var (
	Fixed iso8583.LengthCodec = fixed{}

	LLVarASCII     iso8583.LengthCodec = varLen{name: "len.ll.ascii", digits: 2, repr: reprASCII}
	LLLVarASCII    iso8583.LengthCodec = varLen{name: "len.lll.ascii", digits: 3, repr: reprASCII}
	LLLLVarASCII   iso8583.LengthCodec = varLen{name: "len.llll.ascii", digits: 4, repr: reprASCII}
	LLLLLVarASCII  iso8583.LengthCodec = varLen{name: "len.lllll.ascii", digits: 5, repr: reprASCII}
	LLLLLLVarASCII iso8583.LengthCodec = varLen{name: "len.llllll.ascii", digits: 6, repr: reprASCII}

	LLVarBCD  iso8583.LengthCodec = varLen{name: "len.ll.bcd", digits: 2, repr: reprBCD}
	LLLVarBCD iso8583.LengthCodec = varLen{name: "len.lll.bcd", digits: 3, repr: reprBCD}

	LLVarBinary  iso8583.LengthCodec = varLen{name: "len.ll.bin", bytes: 1, repr: reprBinary}
	LLLVarBinary iso8583.LengthCodec = varLen{name: "len.lll.bin", bytes: 2, repr: reprBinary}

	LLVarEBCDIC  iso8583.LengthCodec = varLen{name: "len.ll.ebcdic", digits: 2, repr: reprEBCDIC}
	LLLVarEBCDIC iso8583.LengthCodec = varLen{name: "len.lll.ebcdic", digits: 3, repr: reprEBCDIC}
)

// All returns every length codec in the catalog, for registry population.
func All() []iso8583.LengthCodec {
	return []iso8583.LengthCodec{
		Fixed,
		LLVarASCII, LLLVarASCII, LLLLVarASCII, LLLLLVarASCII, LLLLLLVarASCII,
		LLVarBCD, LLLVarBCD,
		LLVarBinary, LLLVarBinary,
		LLVarEBCDIC, LLLVarEBCDIC,
	}
}
