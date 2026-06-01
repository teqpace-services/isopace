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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
)

// KeyUsage restricts what a stored key may be used for. The zero value
// (UsageAny) places no restriction.
type KeyUsage uint8

const (
	UsageAny  KeyUsage = iota // no restriction
	UsagePIN                  // PIN encryption / translation
	UsageMAC                  // message authentication
	UsageData                 // data encryption
	UsageKEK                  // key encryption (wrapping)
)

func (u KeyUsage) String() string {
	switch u {
	case UsagePIN:
		return "pin"
	case UsageMAC:
		return "mac"
	case UsageData:
		return "data"
	case UsageKEK:
		return "kek"
	default:
		return "any"
	}
}

// KeyMeta is the non-secret metadata stored alongside a sealed key.
type KeyMeta struct {
	Usage   KeyUsage
	Version int
	Label   string
}

// AuditEvent describes one vault operation for the audit hook.
type AuditEvent struct {
	Op     string // "set-key", "import-key", "encrypt-pin", "translate-pin", "mac", "verify-mac"
	KeyRef string
	OK     bool
	Err    error
}

// SealedVault is a hardened software [Vault]: working keys are stored encrypted
// at rest under a key-encryption key (KEK) with AES-GCM, decrypted only
// transiently for a single operation and zeroized immediately afterwards, with a
// key check value, usage enforcement, and an audit hook. The KEK plaintext is
// not retained by the vault; supply it from a deployment secret (environment,
// file, cloud KMS, or unwrapped from an HSM) and zeroize your copy.
//
// SECURITY: SealedVault raises the bar for software key storage and is suitable
// for development, staging, and non-PIN cryptography (MAC, data encryption, key
// wrapping). It is still software on a general-purpose host: it is NOT a Secure
// Cryptographic Device and cannot satisfy PCI PIN Security or PCI P2PE, which
// require a certified tamper-responsive HSM for PIN translation and the
// acquiring/issuing key hierarchy. Use an HSM-backed Vault adapter for
// production PIN and key handling. (The recovered PIN in TranslatePIN
// necessarily exists in process memory; an HSM never exposes it.)
type SealedVault struct {
	aead  cipher.AEAD
	rand  io.Reader
	audit func(AuditEvent)

	mu   sync.RWMutex
	keys map[string]*sealedKey
}

type sealedKey struct {
	blob []byte // nonce ‖ AES-GCM ciphertext of the key
	kcv  []byte // 3-byte key check value (nil for non-DES/3DES lengths)
	meta KeyMeta
}

var _ Vault = (*SealedVault)(nil)

// SealedOption configures a SealedVault.
type SealedOption func(*SealedVault)

// WithAudit sets a hook called after every operation.
func WithAudit(fn func(AuditEvent)) SealedOption {
	return func(v *SealedVault) { v.audit = fn }
}

// NewSealedVault builds a hardened vault that seals working keys under kek (a
// 16-, 24-, or 32-byte AES key). The KEK is not retained in plaintext.
func NewSealedVault(kek []byte, opts ...SealedOption) (*SealedVault, error) {
	switch len(kek) {
	case 16, 24, 32:
	default:
		return nil, fmt.Errorf("vault: KEK must be 16, 24, or 32 bytes, got %d", len(kek))
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("vault: KEK cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("vault: KEK GCM: %w", err)
	}
	v := &SealedVault{aead: aead, rand: rand.Reader, keys: map[string]*sealedKey{}}
	for _, o := range opts {
		o(v)
	}
	return v, nil
}

// SetKey seals key under the KEK and stores it under ref with the given metadata.
func (v *SealedVault) SetKey(ref string, key []byte, meta KeyMeta) error {
	blob, err := v.seal(key)
	if err != nil {
		v.emit(AuditEvent{Op: "set-key", KeyRef: ref, Err: err})
		return err
	}
	sk := &sealedKey{blob: blob, kcv: KeyCheckValue(key), meta: meta}
	v.mu.Lock()
	v.keys[ref] = sk
	v.mu.Unlock()
	v.emit(AuditEvent{Op: "set-key", KeyRef: ref, OK: true})
	return nil
}

// ImportKeyBlock unwraps a TR-31 key block under kbpk and seals the recovered key
// under ref with the given usage, returning the block header.
func (v *SealedVault) ImportKeyBlock(ref string, kbpk []byte, block string, usage KeyUsage) (KeyBlockHeader, error) {
	hdr, key, err := UnwrapKeyBlock(kbpk, block)
	if err != nil {
		v.emit(AuditEvent{Op: "import-key", KeyRef: ref, Err: err})
		return KeyBlockHeader{}, err
	}
	defer clearBytes(key) // zeroize the recovered key once sealed
	if err := v.SetKey(ref, key, KeyMeta{Usage: usage}); err != nil {
		return hdr, err
	}
	return hdr, nil
}

// KCV returns the stored key check value for ref (nil if absent or unsupported
// key length), without exposing the key.
func (v *SealedVault) KCV(ref string) []byte {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if sk, ok := v.keys[ref]; ok {
		return append([]byte(nil), sk.kcv...)
	}
	return nil
}

func (v *SealedVault) seal(key []byte) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(v.rand, nonce); err != nil {
		return nil, fmt.Errorf("vault: seal nonce: %w", err)
	}
	return v.aead.Seal(nonce, nonce, key, nil), nil // dst=nonce → output is nonce‖ciphertext
}

func (v *SealedVault) open(blob []byte) ([]byte, error) {
	ns := v.aead.NonceSize()
	if len(blob) < ns {
		return nil, fmt.Errorf("vault: sealed key too short")
	}
	key, err := v.aead.Open(nil, blob[:ns], blob[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("vault: unseal: %w", err)
	}
	return key, nil
}

// withKey transiently decrypts the key for ref, enforces its usage, runs fn, and
// zeroizes the plaintext key afterwards.
func (v *SealedVault) withKey(ref string, want KeyUsage, fn func(key []byte) error) error {
	v.mu.RLock()
	sk, ok := v.keys[ref]
	v.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownKey, ref)
	}
	if want != UsageAny && sk.meta.Usage != UsageAny && sk.meta.Usage != want {
		return fmt.Errorf("vault: key %q usage %s not permitted for %s", ref, sk.meta.Usage, want)
	}
	key, err := v.open(sk.blob)
	if err != nil {
		return err
	}
	defer clearBytes(key)
	return fn(key)
}

// EncryptPINBlock implements [Vault].
func (v *SealedVault) EncryptPINBlock(keyRef string, format PINBlockFormat, pin, pan string) ([]byte, error) {
	var out []byte
	err := v.withKey(keyRef, UsagePIN, func(key []byte) error {
		clearBlk, e := EncodePINBlock(format, pin, pan)
		if e != nil {
			return e
		}
		defer clearBytes(clearBlk) // the clear PIN block carries the PIN; wipe it
		b, e := tripleDESBlock(key)
		if e != nil {
			return e
		}
		out, e = ecbEncrypt(b, clearBlk)
		return e
	})
	v.emit(AuditEvent{Op: "encrypt-pin", KeyRef: keyRef, OK: err == nil, Err: err})
	return out, err
}

// TranslatePIN implements [Vault].
func (v *SealedVault) TranslatePIN(srcRef, dstRef string, encBlock []byte, pan string, srcFormat, dstFormat PINBlockFormat) ([]byte, error) {
	var out []byte
	err := v.withKey(srcRef, UsagePIN, func(srcKey []byte) error {
		return v.withKey(dstRef, UsagePIN, func(dstKey []byte) error {
			bs, e := tripleDESBlock(srcKey)
			if e != nil {
				return e
			}
			clr, e := ecbDecrypt(bs, encBlock)
			if e != nil {
				return e
			}
			defer clearBytes(clr) // recovered clear PIN block
			pin, e := DecodePINBlock(srcFormat, clr, pan)
			if e != nil {
				return e
			}
			reblock, e := EncodePINBlock(dstFormat, pin, pan)
			if e != nil {
				return e
			}
			defer clearBytes(reblock) // re-encoded clear PIN block
			bd, e := tripleDESBlock(dstKey)
			if e != nil {
				return e
			}
			out, e = ecbEncrypt(bd, reblock)
			return e
		})
	})
	v.emit(AuditEvent{Op: "translate-pin", KeyRef: srcRef + "->" + dstRef, OK: err == nil, Err: err})
	return out, err
}

// GenerateMAC implements [Vault].
func (v *SealedVault) GenerateMAC(keyRef string, alg MACAlgorithm, pad Padding, data []byte) ([]byte, error) {
	var out []byte
	err := v.withKey(keyRef, UsageMAC, func(key []byte) error {
		var e error
		out, e = GenerateMAC(alg, pad, key, data)
		return e
	})
	v.emit(AuditEvent{Op: "mac", KeyRef: keyRef, OK: err == nil, Err: err})
	return out, err
}

// VerifyMAC implements [Vault].
func (v *SealedVault) VerifyMAC(keyRef string, alg MACAlgorithm, pad Padding, data, mac []byte) (bool, error) {
	var ok bool
	err := v.withKey(keyRef, UsageMAC, func(key []byte) error {
		var e error
		ok, e = VerifyMAC(alg, pad, key, data, mac)
		return e
	})
	v.emit(AuditEvent{Op: "verify-mac", KeyRef: keyRef, OK: err == nil, Err: err})
	return ok, err
}

func (v *SealedVault) emit(e AuditEvent) {
	if v.audit != nil {
		v.audit(e)
	}
}

// KeyCheckValue returns the standard 3-byte key check value for a DES/3DES key
// (the first three bytes of the all-zero block encrypted under the key), or nil
// if the key is not a DES/3DES key length. It identifies a key without exposing
// it — the same value HSMs report on key load.
func KeyCheckValue(key []byte) []byte {
	var b cipher.Block
	var err error
	switch len(key) {
	case 8:
		b, err = desBlock(key)
	case 16, 24:
		b, err = tripleDESBlock(key)
	default:
		return nil
	}
	if err != nil {
		return nil
	}
	return blockEnc(b, make([]byte, 8))[:3]
}

// clearBytes zeroizes a byte slice (best-effort key hygiene).
func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
