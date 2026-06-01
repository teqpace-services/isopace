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
	"crypto/des"
	"fmt"
)

// tripleDESBlock builds a 3DES cipher block from a double-length (16-byte,
// K1‖K2 → expanded to K1‖K2‖K1) or triple-length (24-byte) key — the two key
// lengths used throughout payment cryptography.
func tripleDESBlock(key []byte) (cipher.Block, error) {
	switch len(key) {
	case 24:
		return des.NewTripleDESCipher(key)
	case 16:
		k := make([]byte, 24)
		copy(k, key)
		copy(k[16:], key[:8])
		return des.NewTripleDESCipher(k)
	default:
		return nil, fmt.Errorf("vault: 3DES key must be 16 or 24 bytes, got %d", len(key))
	}
}

// desBlock builds a single-DES cipher block from an 8-byte key.
func desBlock(key []byte) (cipher.Block, error) {
	if len(key) != 8 {
		return nil, fmt.Errorf("vault: DES key must be 8 bytes, got %d", len(key))
	}
	return des.NewCipher(key)
}

// ecbEncrypt encrypts src (a whole number of blocks) under b in ECB mode,
// returning a fresh slice.
func ecbEncrypt(b cipher.Block, src []byte) ([]byte, error) {
	bs := b.BlockSize()
	if len(src)%bs != 0 {
		return nil, fmt.Errorf("vault: ECB input %d not a multiple of block size %d", len(src), bs)
	}
	out := make([]byte, len(src))
	for i := 0; i < len(src); i += bs {
		b.Encrypt(out[i:i+bs], src[i:i+bs])
	}
	return out, nil
}

// ecbDecrypt decrypts src (a whole number of blocks) under b in ECB mode.
func ecbDecrypt(b cipher.Block, src []byte) ([]byte, error) {
	bs := b.BlockSize()
	if len(src)%bs != 0 {
		return nil, fmt.Errorf("vault: ECB input %d not a multiple of block size %d", len(src), bs)
	}
	out := make([]byte, len(src))
	for i := 0; i < len(src); i += bs {
		b.Decrypt(out[i:i+bs], src[i:i+bs])
	}
	return out, nil
}

// cbcEncrypt encrypts src under b in CBC mode with the given IV.
func cbcEncrypt(b cipher.Block, iv, src []byte) ([]byte, error) {
	bs := b.BlockSize()
	if len(iv) != bs {
		return nil, fmt.Errorf("vault: IV %d != block size %d", len(iv), bs)
	}
	if len(src)%bs != 0 {
		return nil, fmt.Errorf("vault: CBC input %d not a multiple of block size %d", len(src), bs)
	}
	out := make([]byte, len(src))
	cipher.NewCBCEncrypter(b, iv).CryptBlocks(out, src)
	return out, nil
}

// cbcDecrypt decrypts src under b in CBC mode with the given IV.
func cbcDecrypt(b cipher.Block, iv, src []byte) ([]byte, error) {
	bs := b.BlockSize()
	if len(iv) != bs {
		return nil, fmt.Errorf("vault: IV %d != block size %d", len(iv), bs)
	}
	if len(src)%bs != 0 {
		return nil, fmt.Errorf("vault: CBC input %d not a multiple of block size %d", len(src), bs)
	}
	out := make([]byte, len(src))
	cipher.NewCBCDecrypter(b, iv).CryptBlocks(out, src)
	return out, nil
}

// blockEnc encrypts a single block under b, returning a fresh slice.
func blockEnc(b cipher.Block, src []byte) []byte {
	out := make([]byte, b.BlockSize())
	b.Encrypt(out, src)
	return out
}

// blockDec decrypts a single block under b, returning a fresh slice.
func blockDec(b cipher.Block, src []byte) []byte {
	out := make([]byte, b.BlockSize())
	b.Decrypt(out, src)
	return out
}

// xorInto writes a⊕b into dst (all equal length); dst may alias a or b.
func xorInto(dst, a, b []byte) {
	for i := range a {
		dst[i] = a[i] ^ b[i]
	}
}

// xored returns a fresh a⊕b.
func xored(a, b []byte) []byte {
	out := make([]byte, len(a))
	xorInto(out, a, b)
	return out
}
