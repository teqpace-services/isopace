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
	"crypto/cipher"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
)

// MACAlgorithm selects an ISO 9797-1 MAC algorithm.
type MACAlgorithm int

const (
	// MACAlg1 is ISO 9797-1 MAC algorithm 1: DES CBC-MAC under a single 8-byte
	// key.
	MACAlg1 MACAlgorithm = iota
	// MACAlg3 is ISO 9797-1 MAC algorithm 3 (the ANSI X9.19 retail MAC): DES
	// CBC-MAC under K1 with a final transform E_K1(D_K2(·)) using a 16-byte
	// double-length key.
	MACAlg3
)

// Padding selects an ISO 9797-1 padding method.
type Padding int

const (
	// Pad1 is padding method 1: append the minimum zero bytes to fill the last
	// block (no padding if already aligned and non-empty).
	Pad1 Padding = iota
	// Pad2 is padding method 2: append 0x80 then zero bytes to fill the last
	// block (always adds at least one byte).
	Pad2
)

// GenerateMAC computes the full 8-byte MAC of data under key. Callers truncate
// to the agreed MAC length (commonly 4 bytes) themselves.
func GenerateMAC(alg MACAlgorithm, pad Padding, key, data []byte) ([]byte, error) {
	padded := applyPadding(pad, data, 8)
	switch alg {
	case MACAlg1:
		b, err := desBlock(key)
		if err != nil {
			return nil, err
		}
		return cbcMAC(b, padded), nil
	case MACAlg3:
		if len(key) != 16 {
			return nil, fmt.Errorf("vault: retail MAC key must be 16 bytes, got %d", len(key))
		}
		b1, err := desBlock(key[:8])
		if err != nil {
			return nil, err
		}
		b2, err := desBlock(key[8:])
		if err != nil {
			return nil, err
		}
		h := cbcMAC(b1, padded)
		return blockEnc(b1, blockDec(b2, h)), nil // E_K1(D_K2(H))
	default:
		return nil, fmt.Errorf("vault: unknown MAC algorithm %d", alg)
	}
}

// VerifyMAC recomputes the MAC over data and constant-time compares its leftmost
// len(mac) bytes to mac. A mac longer than the block or empty is rejected.
func VerifyMAC(alg MACAlgorithm, pad Padding, key, data, mac []byte) (bool, error) {
	full, err := GenerateMAC(alg, pad, key, data)
	if err != nil {
		return false, err
	}
	if len(mac) == 0 || len(mac) > len(full) {
		return false, fmt.Errorf("vault: MAC length %d out of range", len(mac))
	}
	return subtle.ConstantTimeCompare(full[:len(mac)], mac) == 1, nil
}

// SHA256MAC computes SHA-256(key ‖ data) — a prefix-keyed hash MAC used by some
// acquirer links (e.g. CoralPay's DE 128 message hash and the param-download DE
// 64), where key is the session-key bytes and data is the packed message up to the
// MAC field. It is NOT HMAC: it is a plain prefix-keyed digest. Callers hex-encode
// the returned 32 bytes to the 64-character on-wire field (the hex is inherently
// fixed-width, so it matches the legacy zero-left-padded BigInteger form).
func SHA256MAC(key, data []byte) []byte {
	h := sha256.New()
	h.Write(key)
	h.Write(data)
	return h.Sum(nil)
}

// VerifySHA256MAC recomputes SHA256MAC(key, data) and constant-time compares its
// leftmost len(mac) bytes to mac (the full 32 bytes for an untruncated MAC).
func VerifySHA256MAC(key, data, mac []byte) (bool, error) {
	full := SHA256MAC(key, data)
	if len(mac) == 0 || len(mac) > len(full) {
		return false, fmt.Errorf("vault: SHA-256 MAC length %d out of range", len(mac))
	}
	return subtle.ConstantTimeCompare(full[:len(mac)], mac) == 1, nil
}

// cbcMAC returns the final block of a zero-IV CBC encryption of data (a whole
// number of blocks) under b.
func cbcMAC(b cipher.Block, data []byte) []byte {
	bs := b.BlockSize()
	x := make([]byte, bs)
	for i := 0; i < len(data); i += bs {
		x = blockEnc(b, xored(x, data[i:i+bs]))
	}
	return x
}

// applyPadding returns data padded to a multiple of bs by the chosen method.
func applyPadding(pad Padding, data []byte, bs int) []byte {
	switch pad {
	case Pad2:
		out := make([]byte, 0, len(data)+bs)
		out = append(out, data...)
		out = append(out, 0x80)
		for len(out)%bs != 0 {
			out = append(out, 0x00)
		}
		return out
	default: // Pad1
		if len(data) == 0 {
			return make([]byte, bs)
		}
		if len(data)%bs == 0 {
			return append([]byte(nil), data...)
		}
		out := append([]byte(nil), data...)
		for len(out)%bs != 0 {
			out = append(out, 0x00)
		}
		return out
	}
}
