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

import (
	"errors"
	"fmt"
	"sync"
)

// ErrUnknownKey is returned when an operation names a key the vault does not hold.
var ErrUnknownKey = errors.New("vault: unknown key")

// The key-management façade is composed from small capability interfaces so a
// hardware adapter can implement exactly the operations its device supports.
// Operations name keys by reference rather than passing key material, so the
// same calls work whether the keys live in software ([SoftVault]) or in an HSM
// behind an adapter (kept in a separate module so the core stays stdlib-only).
//
// A general-purpose PKCS#11 HSM typically provides [Macer] (and possibly
// [PINEncryptor]); a payment HSM additionally provides [PINTranslator]. Callers
// should depend on the narrowest capability they need and type-assert for it:
//
//	tr, ok := v.(vault.PINTranslator)
//	if !ok { return errors.New("configured vault cannot translate PINs") }

// PINEncryptor enciphers a CLEAR PIN into a PIN block under a device-resident
// key. Because it takes the clear PIN, it is an issuer-side / trusted-context
// operation (e.g. PIN issuance) — at an acquiring switch the clear PIN must
// never be present, so a switch uses [PINTranslator] instead.
type PINEncryptor interface {
	// EncryptPINBlock encodes pin for the format and encrypts the 8-byte block
	// under the named PIN key (3DES ECB).
	EncryptPINBlock(keyRef string, format PINBlockFormat, pin, pan string) ([]byte, error)
}

// PINTranslator re-enciphers an ENCRYPTED PIN block from one key (and format) to
// another — the classic acquirer/switch PIN-translate. The clear PIN does not
// appear in this interface.
//
// Contract: a conforming hardware implementation MUST perform the translation
// atomically inside the device so the clear PIN never leaves it (PCI PIN
// Security). An adapter that cannot translate without exposing the clear PIN in
// host memory (for example one limited to stock PKCS#11, which has no atomic
// translate mechanism) MUST NOT implement this interface — callers rely on its
// presence to mean the operation is PIN-secure.
type PINTranslator interface {
	// TranslatePIN re-enciphers encBlock from srcRef/srcFormat to dstRef/dstFormat.
	TranslatePIN(srcRef, dstRef string, encBlock []byte, pan string, srcFormat, dstFormat PINBlockFormat) ([]byte, error)
}

// Macer generates and verifies message authentication codes under a
// device-resident key; the key material never leaves the device.
type Macer interface {
	// GenerateMAC computes a MAC over data under the named key.
	GenerateMAC(keyRef string, alg MACAlgorithm, pad Padding, data []byte) ([]byte, error)
	// VerifyMAC verifies a MAC over data under the named key.
	VerifyMAC(keyRef string, alg MACAlgorithm, pad Padding, data, mac []byte) (bool, error)
}

// Vault is the full key-management façade: the composition of every capability,
// implemented by the software backend ([SoftVault], [SealedVault]) and by
// full-function payment HSM adapters. An adapter that supports only some
// capabilities should expose those interface types ([Macer], etc.) rather than
// the full Vault.
type Vault interface {
	PINEncryptor
	PINTranslator
	Macer
}

var (
	_ Vault         = (*SoftVault)(nil)
	_ PINEncryptor  = (*SoftVault)(nil)
	_ PINTranslator = (*SoftVault)(nil)
	_ Macer         = (*SoftVault)(nil)
)

// SoftVault is the in-process Vault: keys are held in memory and operations run
// with the Go standard library. It is for development, testing, and conformance
// only — production PIN and key handling must use a certified HSM (see the
// package documentation).
type SoftVault struct {
	mu   sync.RWMutex
	keys map[string][]byte
}

// NewSoftVault returns an empty software vault.
func NewSoftVault() *SoftVault {
	return &SoftVault{keys: map[string][]byte{}}
}

// SetKey loads a clear key under ref (a copy is stored).
func (v *SoftVault) SetKey(ref string, key []byte) {
	v.mu.Lock()
	v.keys[ref] = append([]byte(nil), key...)
	v.mu.Unlock()
}

// ImportKeyBlock unwraps a TR-31 key block under kbpk and stores the recovered
// key under ref, returning the block's header metadata.
func (v *SoftVault) ImportKeyBlock(ref string, kbpk []byte, block string) (KeyBlockHeader, error) {
	hdr, key, err := UnwrapKeyBlock(kbpk, block)
	if err != nil {
		return KeyBlockHeader{}, err
	}
	v.SetKey(ref, key)
	return hdr, nil
}

func (v *SoftVault) key(ref string) ([]byte, error) {
	v.mu.RLock()
	k, ok := v.keys[ref]
	v.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKey, ref)
	}
	return k, nil
}

// EncryptPINBlock implements [Vault].
func (v *SoftVault) EncryptPINBlock(keyRef string, format PINBlockFormat, pin, pan string) ([]byte, error) {
	k, err := v.key(keyRef)
	if err != nil {
		return nil, err
	}
	clear, err := EncodePINBlock(format, pin, pan)
	if err != nil {
		return nil, err
	}
	b, err := tripleDESBlock(k)
	if err != nil {
		return nil, err
	}
	return ecbEncrypt(b, clear)
}

// TranslatePIN implements [Vault].
func (v *SoftVault) TranslatePIN(srcRef, dstRef string, encBlock []byte, pan string, srcFormat, dstFormat PINBlockFormat) ([]byte, error) {
	ks, err := v.key(srcRef)
	if err != nil {
		return nil, err
	}
	kd, err := v.key(dstRef)
	if err != nil {
		return nil, err
	}
	bs, err := tripleDESBlock(ks)
	if err != nil {
		return nil, err
	}
	clear, err := ecbDecrypt(bs, encBlock)
	if err != nil {
		return nil, err
	}
	pin, err := DecodePINBlock(srcFormat, clear, pan)
	if err != nil {
		return nil, err
	}
	reblock, err := EncodePINBlock(dstFormat, pin, pan)
	if err != nil {
		return nil, err
	}
	bd, err := tripleDESBlock(kd)
	if err != nil {
		return nil, err
	}
	return ecbEncrypt(bd, reblock)
}

// GenerateMAC implements [Vault].
func (v *SoftVault) GenerateMAC(keyRef string, alg MACAlgorithm, pad Padding, data []byte) ([]byte, error) {
	k, err := v.key(keyRef)
	if err != nil {
		return nil, err
	}
	return GenerateMAC(alg, pad, k, data)
}

// VerifyMAC implements [Vault].
func (v *SoftVault) VerifyMAC(keyRef string, alg MACAlgorithm, pad Padding, data, mac []byte) (bool, error) {
	k, err := v.key(keyRef)
	if err != nil {
		return false, err
	}
	return VerifyMAC(alg, pad, k, data, mac)
}
