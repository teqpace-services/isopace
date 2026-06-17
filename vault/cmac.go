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

package vault

import "crypto/cipher"

// cmacBlock computes the CMAC (NIST SP 800-38B / RFC 4493, generalised to any
// block size) of msg under b. It is used internally by the TR-31 version B key
// block, whose authentication tag is a 3DES CMAC.
func cmacBlock(b cipher.Block, msg []byte) []byte {
	bs := b.BlockSize()
	k1, k2 := cmacSubkeys(b)

	var last []byte
	if len(msg) > 0 && len(msg)%bs == 0 {
		last = xored(msg[len(msg)-bs:], k1)
		msg = msg[:len(msg)-bs]
	} else {
		rem := len(msg) % bs
		block := make([]byte, bs)
		copy(block, msg[len(msg)-rem:])
		block[rem] = 0x80
		last = xored(block, k2)
		msg = msg[:len(msg)-rem]
	}

	x := make([]byte, bs)
	for i := 0; i < len(msg); i += bs {
		x = blockEnc(b, xored(x, msg[i:i+bs]))
	}
	return blockEnc(b, xored(x, last))
}

// cmacSubkeys derives the two CMAC subkeys K1, K2 from the cipher.
func cmacSubkeys(b cipher.Block) (k1, k2 []byte) {
	bs := b.BlockSize()
	l := blockEnc(b, make([]byte, bs))
	k1 = dbl(l)
	k2 = dbl(k1)
	return k1, k2
}

// dbl left-shifts a block by one bit and conditionally XORs the constant Rb
// (0x1B for 64-bit blocks, 0x87 for 128-bit) when the top bit was set.
func dbl(in []byte) []byte {
	bs := len(in)
	rb := byte(0x1B)
	if bs == 16 {
		rb = 0x87
	}
	msb := in[0] >> 7
	out := make([]byte, bs)
	for i := 0; i < bs-1; i++ {
		out[i] = in[i]<<1 | in[i+1]>>7
	}
	out[bs-1] = in[bs-1] << 1
	if msb == 1 {
		out[bs-1] ^= rb
	}
	return out
}
