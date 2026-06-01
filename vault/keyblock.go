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
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// KeyBlockHeader is the cleartext metadata of a TR-31 key block: what the
// wrapped key is for and how it may be used. The version is fixed to 'B' (the
// 3DES, CMAC-bound variant) by this implementation.
type KeyBlockHeader struct {
	Version       byte   // always 'B' here
	KeyUsage      string // 2 chars, e.g. "P0" (PIN), "M0" (MAC), "B0" (BDK), "D0" (data)
	Algorithm     byte   // wrapped-key algorithm: 'T' 3DES, 'A' AES, 'D' DES, ...
	ModeOfUse     byte   // 'E' encrypt, 'D' decrypt, 'B' both, 'N' none, ...
	KeyVersion    string // 2 chars, "00" if unused
	Exportability byte   // 'E' exportable, 'N' non-exportable, 'S' sensitive
}

// WrapKeyBlock wraps key under the Key Block Protection Key kbpk (16- or 24-byte
// 3DES) as a TR-31 version B key block, drawing pad randomness from crypto/rand.
func WrapKeyBlock(kbpk []byte, hdr KeyBlockHeader, key []byte) (string, error) {
	return WrapKeyBlockRand(kbpk, hdr, key, rand.Reader)
}

// WrapKeyBlockRand is [WrapKeyBlock] with an explicit randomness source.
func WrapKeyBlockRand(kbpk []byte, hdr KeyBlockHeader, key []byte, r io.Reader) (string, error) {
	if len(kbpk) != 16 && len(kbpk) != 24 {
		return "", fmt.Errorf("vault: KBPK must be 16 or 24 bytes, got %d", len(kbpk))
	}
	if len(key) == 0 {
		return "", fmt.Errorf("vault: empty key")
	}

	// Confidential data = key length in bits (2 bytes) ‖ key ‖ random pad to a
	// multiple of the 8-byte block.
	plain := make([]byte, 2+len(key))
	binary.BigEndian.PutUint16(plain[:2], uint16(len(key)*8))
	copy(plain[2:], key)
	if pad := (8 - len(plain)%8) % 8; pad > 0 {
		rb := make([]byte, pad)
		if _, err := io.ReadFull(r, rb); err != nil {
			return "", fmt.Errorf("vault: read key-block pad: %w", err)
		}
		plain = append(plain, rb...)
	}

	// Total length (characters): header + hex(cipher) + hex(MAC). cipher is the
	// same length as plain; the version B MAC is one 8-byte block.
	blockLen := 16 + 2*len(plain) + 16
	header, err := hdr.marshal(blockLen)
	if err != nil {
		return "", err
	}

	kbek, kbmk, err := tr31DeriveKeys(kbpk)
	if err != nil {
		return "", err
	}
	bmac, err := tripleDESBlock(kbmk)
	if err != nil {
		return "", err
	}
	mac := cmacBlock(bmac, append(append([]byte(nil), header...), plain...))

	benc, err := tripleDESBlock(kbek)
	if err != nil {
		return "", err
	}
	ct, err := cbcEncrypt(benc, mac, plain) // MAC doubles as the CBC IV
	if err != nil {
		return "", err
	}
	return string(header) + upperHex(ct) + upperHex(mac), nil
}

// UnwrapKeyBlock authenticates and decrypts a TR-31 version B key block under
// kbpk, returning the header metadata and the recovered key. It fails if the MAC
// does not verify (wrong KBPK or tampering).
func UnwrapKeyBlock(kbpk []byte, block string) (KeyBlockHeader, []byte, error) {
	var zero KeyBlockHeader
	if len(kbpk) != 16 && len(kbpk) != 24 {
		return zero, nil, fmt.Errorf("vault: KBPK must be 16 or 24 bytes, got %d", len(kbpk))
	}
	if len(block) < 16 {
		return zero, nil, fmt.Errorf("vault: key block too short")
	}
	header := []byte(block[:16])
	hdr, err := parseHeader(header)
	if err != nil {
		return zero, nil, err
	}
	if hdr.Version != 'B' {
		return zero, nil, fmt.Errorf("vault: unsupported key block version %q (only B)", hdr.Version)
	}
	blockLen, err := atoiFixed(block[1:5])
	if err != nil {
		return zero, nil, fmt.Errorf("vault: bad key block length: %w", err)
	}
	if blockLen != len(block) {
		return zero, nil, fmt.Errorf("vault: key block length %d != actual %d", blockLen, len(block))
	}
	if block[12:14] != "00" {
		return zero, nil, fmt.Errorf("vault: optional blocks not supported")
	}

	body := block[16:]
	if len(body) < 16 || len(body)%16 != 0 {
		return zero, nil, fmt.Errorf("vault: malformed key block body")
	}
	ct, err := hex.DecodeString(body[:len(body)-16])
	if err != nil {
		return zero, nil, fmt.Errorf("vault: decode key data: %w", err)
	}
	mac, err := hex.DecodeString(body[len(body)-16:])
	if err != nil {
		return zero, nil, fmt.Errorf("vault: decode MAC: %w", err)
	}

	kbek, kbmk, err := tr31DeriveKeys(kbpk)
	if err != nil {
		return zero, nil, err
	}
	benc, err := tripleDESBlock(kbek)
	if err != nil {
		return zero, nil, err
	}
	plain, err := cbcDecrypt(benc, mac, ct)
	if err != nil {
		return zero, nil, err
	}

	bmac, err := tripleDESBlock(kbmk)
	if err != nil {
		return zero, nil, err
	}
	want := cmacBlock(bmac, append(append([]byte(nil), header...), plain...))
	if subtle.ConstantTimeCompare(want, mac) != 1 {
		return zero, nil, fmt.Errorf("vault: key block authentication failed")
	}

	if len(plain) < 2 {
		return zero, nil, fmt.Errorf("vault: key block plaintext too short")
	}
	keyLen := int(binary.BigEndian.Uint16(plain[:2])) / 8
	if keyLen < 0 || 2+keyLen > len(plain) {
		return zero, nil, fmt.Errorf("vault: key length %d exceeds key block", keyLen)
	}
	return hdr, append([]byte(nil), plain[2:2+keyLen]...), nil
}

// tr31DeriveKeys derives the key-block encryption and MAC keys from the KBPK via
// the version B CMAC-based KDF (SP 800-108 counter mode).
func tr31DeriveKeys(kbpk []byte) (kbek, kbmk []byte, err error) {
	b, err := tripleDESBlock(kbpk)
	if err != nil {
		return nil, nil, err
	}
	nbits := len(kbpk) * 8
	iters := len(kbpk) / 8
	algo := uint16(0x0000) // 2-key TDES
	if len(kbpk) == 24 {
		algo = 0x0001 // 3-key TDES
	}
	return tr31kdf(b, 0x0000, algo, nbits, iters), tr31kdf(b, 0x0001, algo, nbits, iters), nil
}

// tr31kdf runs the counter-mode CMAC KDF, concatenating iters 8-byte outputs.
func tr31kdf(b cipher.Block, usage, algo uint16, nbits, iters int) []byte {
	out := make([]byte, 0, iters*8)
	for ctr := 1; ctr <= iters; ctr++ {
		data := []byte{
			byte(ctr),
			byte(usage >> 8), byte(usage),
			0x00,
			byte(algo >> 8), byte(algo),
			byte(nbits >> 8), byte(nbits),
		}
		out = append(out, cmacBlock(b, data)...)
	}
	return out
}

func (h KeyBlockHeader) marshal(blockLen int) ([]byte, error) {
	if len(h.KeyUsage) != 2 {
		return nil, fmt.Errorf("vault: key usage must be 2 chars, got %q", h.KeyUsage)
	}
	kv := h.KeyVersion
	if kv == "" {
		kv = "00"
	}
	if len(kv) != 2 {
		return nil, fmt.Errorf("vault: key version must be 2 chars, got %q", kv)
	}
	if blockLen > 9999 {
		return nil, fmt.Errorf("vault: key block too large (%d)", blockLen)
	}
	buf := make([]byte, 16)
	buf[0] = 'B'
	copy(buf[1:5], fmt.Sprintf("%04d", blockLen))
	copy(buf[5:7], h.KeyUsage)
	buf[7] = h.Algorithm
	buf[8] = h.ModeOfUse
	copy(buf[9:11], kv)
	buf[11] = h.Exportability
	copy(buf[12:14], "00") // no optional blocks
	copy(buf[14:16], "00") // reserved
	return buf, nil
}

func parseHeader(b []byte) (KeyBlockHeader, error) {
	if len(b) < 16 {
		return KeyBlockHeader{}, fmt.Errorf("vault: header too short")
	}
	return KeyBlockHeader{
		Version:       b[0],
		KeyUsage:      string(b[5:7]),
		Algorithm:     b[7],
		ModeOfUse:     b[8],
		KeyVersion:    string(b[9:11]),
		Exportability: b[11],
	}, nil
}

func upperHex(b []byte) string { return strings.ToUpper(hex.EncodeToString(b)) }

func atoiFixed(s string) (int, error) {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("non-digit %q", s[i])
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, nil
}
