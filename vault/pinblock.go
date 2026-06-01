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

package vault

import (
	"crypto/rand"
	"fmt"
	"io"
)

// PINBlockFormat selects an ISO 9564-1 PIN block format.
type PINBlockFormat int

const (
	// ISO0 is format 0 (ANSI X9.8): PIN field XOR PAN field, fixed 'F' padding.
	ISO0 PINBlockFormat = iota
	// ISO1 is format 1: PIN field with random padding and no PAN, for use when
	// the PAN is unavailable.
	ISO1
	// ISO3 is format 3: like format 0 but with random 'A'–'F' padding.
	ISO3
)

func (f PINBlockFormat) control() (byte, bool) {
	switch f {
	case ISO0:
		return 0x0, true
	case ISO1:
		return 0x1, true
	case ISO3:
		return 0x3, true
	default:
		return 0, false
	}
}

// EncodePINBlock builds the clear 8-byte PIN block for the format, drawing any
// random padding from crypto/rand. The result is the *clear* block; encrypt it
// under a PIN key (see [Vault]) before transmission.
func EncodePINBlock(format PINBlockFormat, pin, pan string) ([]byte, error) {
	return EncodePINBlockRand(format, pin, pan, rand.Reader)
}

// EncodePINBlockRand is [EncodePINBlock] with an explicit randomness source, for
// deterministic testing.
func EncodePINBlockRand(format PINBlockFormat, pin, pan string, r io.Reader) ([]byte, error) {
	pf, err := pinField(format, pin, r)
	if err != nil {
		return nil, err
	}
	switch format {
	case ISO0, ISO3:
		af, err := panField(pan)
		if err != nil {
			return nil, err
		}
		return xored(pf, af), nil
	case ISO1:
		return pf, nil
	default:
		return nil, fmt.Errorf("vault: unknown PIN block format %d", format)
	}
}

// DecodePINBlock recovers the PIN from a clear 8-byte PIN block (decrypt it
// first). pan is ignored for formats that do not use it.
func DecodePINBlock(format PINBlockFormat, block []byte, pan string) (string, error) {
	if len(block) != 8 {
		return "", fmt.Errorf("vault: PIN block must be 8 bytes, got %d", len(block))
	}
	ctrl, ok := format.control()
	if !ok {
		return "", fmt.Errorf("vault: unknown PIN block format %d", format)
	}
	clear := block
	if format == ISO0 || format == ISO3 {
		af, err := panField(pan)
		if err != nil {
			return "", err
		}
		clear = xored(block, af)
	}
	nibbles := unpackNibbles(clear)
	if nibbles[0] != ctrl {
		return "", fmt.Errorf("vault: PIN block control nibble %X != format %d", nibbles[0], format)
	}
	l := int(nibbles[1])
	if l < 4 || l > 12 {
		return "", fmt.Errorf("vault: PIN length nibble %d out of range", l)
	}
	pin := make([]byte, l)
	for i := 0; i < l; i++ {
		d := nibbles[2+i]
		if d > 9 {
			return "", fmt.Errorf("vault: non-decimal PIN digit %X", d)
		}
		pin[i] = '0' + d
	}
	if err := checkPadding(format, nibbles[2+l:]); err != nil {
		return "", err
	}
	return string(pin), nil
}

// checkPadding validates the trailing pad nibbles against the format's scheme,
// so a tampered or malformed block is rejected rather than silently accepted
// (ISO 9564-1 block integrity). Format 1 padding is unrestricted.
func checkPadding(format PINBlockFormat, pad []byte) error {
	switch format {
	case ISO0:
		for _, n := range pad {
			if n != 0xF {
				return fmt.Errorf("vault: ISO-0 PIN block has non-F pad nibble %X", n)
			}
		}
	case ISO3:
		for _, n := range pad {
			if n < 0xA {
				return fmt.Errorf("vault: ISO-3 PIN block pad nibble %X below A", n)
			}
		}
	}
	return nil
}

// pinField assembles the 8-byte PIN field for the format.
func pinField(format PINBlockFormat, pin string, r io.Reader) ([]byte, error) {
	ctrl, ok := format.control()
	if !ok {
		return nil, fmt.Errorf("vault: unknown PIN block format %d", format)
	}
	l := len(pin)
	if l < 4 || l > 12 {
		return nil, fmt.Errorf("vault: PIN length %d out of range (4-12)", l)
	}
	nibbles := make([]byte, 16)
	nibbles[0] = ctrl
	nibbles[1] = byte(l)
	for i := 0; i < l; i++ {
		c := pin[i]
		if c < '0' || c > '9' {
			return nil, fmt.Errorf("vault: non-decimal PIN digit %q", c)
		}
		nibbles[2+i] = c - '0'
	}
	if err := padNibbles(format, nibbles[2+l:], r); err != nil {
		return nil, err
	}
	return packNibbles(nibbles), nil
}

// padNibbles fills the tail with the format's padding.
func padNibbles(format PINBlockFormat, tail []byte, r io.Reader) error {
	switch format {
	case ISO0:
		for i := range tail {
			tail[i] = 0xF
		}
		return nil
	case ISO1, ISO3:
		if len(tail) == 0 {
			return nil
		}
		buf := make([]byte, len(tail))
		if _, err := io.ReadFull(r, buf); err != nil {
			return fmt.Errorf("vault: read PIN pad randomness: %w", err)
		}
		for i, b := range buf {
			if format == ISO3 {
				tail[i] = 0xA + b%6 // 0xA–0xF
			} else {
				tail[i] = b & 0x0F // 0x0–0xF
			}
		}
		return nil
	default:
		return fmt.Errorf("vault: unknown PIN block format %d", format)
	}
}

// panField assembles the 8-byte PAN field: 0000 followed by the 12 rightmost
// PAN digits excluding the check digit.
func panField(pan string) ([]byte, error) {
	for i := 0; i < len(pan); i++ {
		if pan[i] < '0' || pan[i] > '9' {
			return nil, fmt.Errorf("vault: non-decimal PAN digit %q", pan[i])
		}
	}
	if len(pan) < 13 {
		return nil, fmt.Errorf("vault: PAN must be at least 13 digits, got %d", len(pan))
	}
	acct := pan[:len(pan)-1] // drop the check digit
	acct = acct[len(acct)-12:]
	field := make([]byte, 16)
	// nibbles 0-3 are zero; 4-15 are the 12 account digits.
	for i := 0; i < 12; i++ {
		field[4+i] = acct[i] - '0'
	}
	return packNibbles(field), nil
}

// packNibbles packs 16 nibbles (high nibble first) into 8 bytes.
func packNibbles(n []byte) []byte {
	out := make([]byte, 8)
	for i := 0; i < 8; i++ {
		out[i] = n[2*i]<<4 | n[2*i+1]&0x0F
	}
	return out
}

// unpackNibbles expands 8 bytes into 16 nibbles (high nibble first).
func unpackNibbles(b []byte) []byte {
	out := make([]byte, 16)
	for i, x := range b {
		out[2*i] = x >> 4
		out[2*i+1] = x & 0x0F
	}
	return out
}
