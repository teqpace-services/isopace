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

import "fmt"

// DUKPT (ANSI X9.24-1, TDES variant) derives a unique key per transaction from a
// Base Derivation Key (BDK) and a Key Serial Number (KSN), so a captured key
// compromises only one transaction. The flow is:
//
//	IPEK    = DeriveIPEK(bdk, ksn)            // one-time, loaded into the device
//	txKey   = DeriveSessionKey(ipek, ksn)     // the per-transaction derived key
//
// In classic X9.24-1 the transaction key is used directly as the PIN encryption
// key (this is what the published test vector pins). The later variant scheme
// (X9.24-1:2009 Annex A) instead derives purpose-specific working keys as
// txKey ⊕ a variant constant; [DeriveVariant] with [VariantPIN] (or another
// constant from the standard) applies it. VariantPIN is the only constant
// exported here, since the others are not pinned by the validated vector.

// keyVariantMask is the X9.24 "C0C0" constant used both in IPEK derivation and
// in each non-reversible key-generation step.
var keyVariantMask = []byte{
	0xC0, 0xC0, 0xC0, 0xC0, 0x00, 0x00, 0x00, 0x00,
	0xC0, 0xC0, 0xC0, 0xC0, 0x00, 0x00, 0x00, 0x00,
}

// VariantPIN is the X9.24-1 variant constant for the PIN encryption working key.
var VariantPIN = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF,
}

// DeriveIPEK derives the Initial PIN Encryption Key from a double-length (16
// byte) BDK and a 10-byte KSN. The KSN's transaction counter is ignored.
func DeriveIPEK(bdk, ksn []byte) ([]byte, error) {
	if len(bdk) != 16 {
		return nil, fmt.Errorf("vault: DUKPT BDK must be 16 bytes, got %d", len(bdk))
	}
	if len(ksn) != 10 {
		return nil, fmt.Errorf("vault: KSN must be 10 bytes, got %d", len(ksn))
	}
	// Base = leftmost 8 bytes of the KSN with the (21-bit) counter cleared. Only
	// the low 5 bits of byte 7 fall inside the leftmost 8 bytes.
	base := make([]byte, 8)
	copy(base, ksn[:8])
	base[7] &= 0xE0

	b1, err := tripleDESBlock(bdk)
	if err != nil {
		return nil, err
	}
	left := blockEnc(b1, base)

	b2, err := tripleDESBlock(xored(bdk, keyVariantMask))
	if err != nil {
		return nil, err
	}
	right := blockEnc(b2, base)

	return append(left, right...), nil
}

// DeriveSessionKey derives the per-transaction key from a 16-byte IPEK and the
// 10-byte KSN, walking the 21-bit transaction counter through the X9.24
// non-reversible key-generation process. In classic DUKPT this key is used
// directly for PIN encryption; under the variant scheme apply [DeriveVariant].
func DeriveSessionKey(ipek, ksn []byte) ([]byte, error) {
	if len(ipek) != 16 {
		return nil, fmt.Errorf("vault: IPEK must be 16 bytes, got %d", len(ipek))
	}
	if len(ksn) != 10 {
		return nil, fmt.Errorf("vault: KSN must be 10 bytes, got %d", len(ksn))
	}
	counter := ksnCounter(ksn)

	// Register = rightmost 8 bytes of the KSN with the counter cleared.
	reg := make([]byte, 8)
	copy(reg, ksn[2:10])
	reg[5] &= 0xE0
	reg[6] = 0
	reg[7] = 0

	curKey := append([]byte(nil), ipek...)
	for shift := uint32(0x100000); shift > 0; shift >>= 1 {
		if counter&shift != 0 {
			setCounterBit(reg, shift)
			ck, err := dukptGenerateKey(curKey, reg)
			if err != nil {
				return nil, err
			}
			curKey = ck
		}
	}
	return curKey, nil
}

// DeriveVariant returns txKey ⊕ mask, the X9.24:2009 variant-scheme way of
// turning a derived transaction key into a purpose-specific working key. mask
// must be 16 bytes (e.g. [VariantPIN]).
func DeriveVariant(txKey, mask []byte) ([]byte, error) {
	if len(txKey) != 16 || len(mask) != 16 {
		return nil, fmt.Errorf("vault: variant needs 16-byte key and mask, got %d/%d", len(txKey), len(mask))
	}
	return xored(txKey, mask), nil
}

// KSNCounter returns the 21-bit transaction counter from a 10-byte KSN.
func KSNCounter(ksn []byte) (uint32, error) {
	if len(ksn) != 10 {
		return 0, fmt.Errorf("vault: KSN must be 10 bytes, got %d", len(ksn))
	}
	return ksnCounter(ksn), nil
}

func ksnCounter(ksn []byte) uint32 {
	return uint32(ksn[7]&0x1F)<<16 | uint32(ksn[8])<<8 | uint32(ksn[9])
}

// setCounterBit sets the bit identified by shift (a single bit in 0..0x1FFFFF)
// into the low 21 bits of the 8-byte register.
func setCounterBit(reg []byte, shift uint32) {
	reg[5] |= byte(shift>>16) & 0x1F
	reg[6] |= byte(shift >> 8)
	reg[7] |= byte(shift)
}

// dukptGenerateKey performs one non-reversible key-generation step: the right
// half from the current key, the left half from the C0C0-masked key.
func dukptGenerateKey(key, reg []byte) ([]byte, error) {
	right, err := dukptEncRegister(key, reg)
	if err != nil {
		return nil, err
	}
	left, err := dukptEncRegister(xored(key, keyVariantMask), reg)
	if err != nil {
		return nil, err
	}
	return append(left, right...), nil
}

// dukptEncRegister computes (DES_encrypt(key[:8], reg ⊕ key[8:]) ) ⊕ key[8:].
func dukptEncRegister(key, reg []byte) ([]byte, error) {
	b, err := desBlock(key[:8])
	if err != nil {
		return nil, err
	}
	bottom := xored(reg, key[8:16])
	bottom = blockEnc(b, bottom)
	return xored(bottom, key[8:16]), nil
}
