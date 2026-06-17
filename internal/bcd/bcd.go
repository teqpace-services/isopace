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

// Package bcd implements packed binary-coded-decimal primitives: two decimal
// digits per byte, high nibble first. Numeric ISO-8583 fields in BCD encoding
// build on these. An odd digit count is left-padded with a single zero nibble
// (the high nibble of the first byte), the conventional numeric-BCD layout.
package bcd

import "errors"

// Errors returned by the BCD primitives.
var (
	ErrNonDigit  = errors.New("bcd: non-decimal digit in input")
	ErrBadNibble = errors.New("bcd: invalid nibble in packed data")
	ErrShort     = errors.New("bcd: packed buffer too short for digit count")
	ErrNegCount  = errors.New("bcd: negative digit count")
)

// PackedLen returns the number of bytes needed to hold n packed digits.
func PackedLen(n int) int { return (n + 1) / 2 }

// Pack encodes ASCII decimal digits as packed BCD. With an odd number of digits
// the result is left-padded with a zero nibble. Pack([]byte("12345")) yields
// {0x01, 0x23, 0x45}.
func Pack(digits []byte) ([]byte, error) {
	n := len(digits)
	out := make([]byte, PackedLen(n))
	oi, di := 0, 0
	if n%2 == 1 {
		d, err := nibble(digits[0])
		if err != nil {
			return nil, err
		}
		out[0] = d // high nibble 0 (pad), low nibble = first digit
		di, oi = 1, 1
	}
	for di < n {
		hi, err := nibble(digits[di])
		if err != nil {
			return nil, err
		}
		lo, err := nibble(digits[di+1])
		if err != nil {
			return nil, err
		}
		out[oi] = hi<<4 | lo
		di += 2
		oi++
	}
	return out, nil
}

// Unpack decodes packed BCD into exactly n ASCII decimal digits, dropping the
// leading pad nibble when n is odd. It validates that every emitted nibble is a
// decimal digit.
func Unpack(b []byte, n int) ([]byte, error) {
	switch {
	case n < 0:
		return nil, ErrNegCount
	case PackedLen(n) > len(b):
		return nil, ErrShort
	}
	out := make([]byte, n)
	used := PackedLen(n)
	oi, start := 0, 0
	if n%2 == 1 {
		lo := b[0] & 0x0F
		if lo > 9 {
			return nil, ErrBadNibble
		}
		out[0] = '0' + lo
		oi, start = 1, 1
	}
	for i := start; i < used; i++ {
		hi, lo := b[i]>>4, b[i]&0x0F
		if hi > 9 || lo > 9 {
			return nil, ErrBadNibble
		}
		out[oi] = '0' + hi
		out[oi+1] = '0' + lo
		oi += 2
	}
	return out, nil
}

// PackRight encodes ASCII decimal digits as right-aligned packed BCD: with an
// odd number of digits the result is right-padded with a zero nibble (the low
// nibble of the last byte). PackRight([]byte("12345")) yields {0x12, 0x34, 0x50}.
func PackRight(digits []byte) ([]byte, error) {
	n := len(digits)
	out := make([]byte, PackedLen(n))
	oi := 0
	for di := 0; di < n; di += 2 {
		hi, err := nibble(digits[di])
		if err != nil {
			return nil, err
		}
		var lo byte
		if di+1 < n {
			if lo, err = nibble(digits[di+1]); err != nil {
				return nil, err
			}
		}
		out[oi] = hi<<4 | lo // trailing low nibble stays 0 when n is odd
		oi++
	}
	return out, nil
}

// UnpackRight decodes right-aligned packed BCD into exactly n ASCII decimal
// digits, dropping the trailing pad nibble when n is odd.
func UnpackRight(b []byte, n int) ([]byte, error) {
	switch {
	case n < 0:
		return nil, ErrNegCount
	case PackedLen(n) > len(b):
		return nil, ErrShort
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		var nib byte
		if i%2 == 0 {
			nib = b[i/2] >> 4
		} else {
			nib = b[i/2] & 0x0F
		}
		if nib > 9 {
			return nil, ErrBadNibble
		}
		out[i] = '0' + nib
	}
	return out, nil
}

func nibble(c byte) (byte, error) {
	if c < '0' || c > '9' {
		return 0, ErrNonDigit
	}
	return c - '0', nil
}
