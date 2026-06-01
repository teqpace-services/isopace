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
	"bytes"
	"errors"
	"sync"
	"testing"
)

func newSealed(t *testing.T, opts ...SealedOption) *SealedVault {
	t.Helper()
	kek := mustHex(t, "000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F") // 32-byte AES-256 KEK
	v, err := NewSealedVault(kek, opts...)
	if err != nil {
		t.Fatalf("NewSealedVault: %v", err)
	}
	return v
}

func TestSealedVaultKEKLength(t *testing.T) {
	if _, err := NewSealedVault(make([]byte, 10)); err == nil {
		t.Error("accepted a 10-byte KEK")
	}
	for _, n := range []int{16, 24, 32} {
		if _, err := NewSealedVault(make([]byte, n)); err != nil {
			t.Errorf("rejected a %d-byte KEK: %v", n, err)
		}
	}
}

func TestSealedVaultKeysEncryptedAtRest(t *testing.T) {
	v := newSealed(t)
	key := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	if err := v.SetKey("zpk", key, KeyMeta{Usage: UsagePIN}); err != nil {
		t.Fatal(err)
	}
	// The stored blob must not contain the plaintext key.
	v.mu.RLock()
	blob := v.keys["zpk"].blob
	v.mu.RUnlock()
	if bytes.Contains(blob, key) {
		t.Error("plaintext key found in the sealed blob")
	}
	// A fresh nonce per seal: re-sealing the same key gives a different blob.
	_ = v.SetKey("zpk2", key, KeyMeta{Usage: UsagePIN})
	v.mu.RLock()
	blob2 := v.keys["zpk2"].blob
	v.mu.RUnlock()
	if bytes.Equal(blob, blob2) {
		t.Error("two seals of the same key produced identical blobs (nonce reuse?)")
	}
	// KCV is the standard zero-block 3DES check value, exposed without the key.
	if got := v.KCV("zpk"); len(got) != 3 || !bytes.Equal(got, KeyCheckValue(key)) {
		t.Errorf("KCV = %X want %X", got, KeyCheckValue(key))
	}
}

func TestSealedVaultPINParityWithSoft(t *testing.T) {
	key := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	pan := "4012345678909"

	soft := NewSoftVault()
	soft.SetKey("zpk", key)
	sealed := newSealed(t)
	if err := sealed.SetKey("zpk", key, KeyMeta{Usage: UsagePIN}); err != nil {
		t.Fatal(err)
	}

	// Encrypting a PIN block under the same key must match SoftVault exactly
	// (the sealing is transparent to the cryptographic result).
	a, err := soft.EncryptPINBlock("zpk", ISO0, "1234", pan)
	if err != nil {
		t.Fatal(err)
	}
	b, err := sealed.EncryptPINBlock("zpk", ISO0, "1234", pan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("sealed PIN block %X != soft %X", b, a)
	}

	// Translate sealed->sealed and recover the PIN.
	sealed.SetKey("zpk2", mustHex(t, "FEDCBA98765432100123456789ABCDEF"), KeyMeta{Usage: UsagePIN})
	trans, err := sealed.TranslatePIN("zpk", "zpk2", b, pan, ISO0, ISO0)
	if err != nil {
		t.Fatalf("TranslatePIN: %v", err)
	}
	if bytes.Equal(trans, b) {
		t.Error("translation under a different key produced identical ciphertext")
	}
}

func TestSealedVaultMAC(t *testing.T) {
	v := newSealed(t)
	key := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	if err := v.SetKey("mac", key, KeyMeta{Usage: UsageMAC}); err != nil {
		t.Fatal(err)
	}
	data := []byte("authorize me")
	mac, err := v.GenerateMAC("mac", MACAlg3, Pad2, data)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := v.VerifyMAC("mac", MACAlg3, Pad2, data, mac)
	if err != nil || !ok {
		t.Errorf("VerifyMAC = %v,%v want true", ok, err)
	}
	// Must match the standalone primitive.
	want, _ := GenerateMAC(MACAlg3, Pad2, key, data)
	if !bytes.Equal(mac, want) {
		t.Errorf("sealed MAC %X != primitive %X", mac, want)
	}
}

func TestSealedVaultUsageEnforcement(t *testing.T) {
	v := newSealed(t)
	key := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	// A MAC-only key may not be used for PIN encryption.
	v.SetKey("maconly", key, KeyMeta{Usage: UsageMAC})
	if _, err := v.EncryptPINBlock("maconly", ISO0, "1234", "4012345678909"); err == nil {
		t.Error("MAC-only key was allowed for PIN encryption")
	}
	// A PIN-only key may not be used for MAC.
	v.SetKey("pinonly", key, KeyMeta{Usage: UsagePIN})
	if _, err := v.GenerateMAC("pinonly", MACAlg3, Pad2, []byte("x")); err == nil {
		t.Error("PIN-only key was allowed for MAC")
	}
	// An unrestricted (UsageAny) key works for both.
	v.SetKey("any", key, KeyMeta{})
	if _, err := v.GenerateMAC("any", MACAlg3, Pad2, []byte("x")); err != nil {
		t.Errorf("UsageAny key rejected for MAC: %v", err)
	}
	// Unknown key.
	if _, err := v.GenerateMAC("ghost", MACAlg3, Pad2, []byte("x")); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("unknown key err = %v want ErrUnknownKey", err)
	}
}

func TestSealedVaultWrongKEKCannotUnseal(t *testing.T) {
	key := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	v1 := newSealed(t)
	v1.SetKey("k", key, KeyMeta{Usage: UsageMAC})

	// A second vault with a different KEK that adopts v1's sealed blob must fail
	// to unseal (GCM authentication), not silently use wrong key material.
	v2, _ := NewSealedVault(make([]byte, 32)) // all-zero KEK, different from v1
	v1.mu.RLock()
	blob := append([]byte(nil), v1.keys["k"].blob...)
	v1.mu.RUnlock()
	v2.mu.Lock()
	v2.keys["k"] = &sealedKey{blob: blob, meta: KeyMeta{Usage: UsageMAC}}
	v2.mu.Unlock()
	if _, err := v2.GenerateMAC("k", MACAlg3, Pad2, []byte("x")); err == nil {
		t.Error("wrong KEK unsealed the key (GCM auth bypassed)")
	}
}

func TestSealedVaultAuditAndImport(t *testing.T) {
	var mu sync.Mutex
	var events []AuditEvent
	v := newSealed(t, WithAudit(func(e AuditEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}))

	// Import a key from a TR-31 block and use it; audit must record both.
	kbpk := mustHex(t, "89E88CF7931444F334BD7547FC3F380C")
	work := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	block, err := WrapKeyBlock(kbpk, KeyBlockHeader{KeyUsage: "M0", Algorithm: 'T', ModeOfUse: 'B', Exportability: 'N'}, work)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.ImportKeyBlock("imported", kbpk, block, UsageMAC); err != nil {
		t.Fatalf("ImportKeyBlock: %v", err)
	}
	mac, err := v.GenerateMAC("imported", MACAlg3, Pad2, []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := GenerateMAC(MACAlg3, Pad2, work, []byte("hi")); !bytes.Equal(mac, want) {
		t.Errorf("imported-key MAC mismatch")
	}

	mu.Lock()
	defer mu.Unlock()
	var sawImport, sawMAC bool
	for _, e := range events {
		if e.Op == "set-key" && e.KeyRef == "imported" && e.OK {
			sawImport = true
		}
		if e.Op == "mac" && e.KeyRef == "imported" && e.OK {
			sawMAC = true
		}
	}
	if !sawImport || !sawMAC {
		t.Errorf("audit missing events: import=%v mac=%v (events=%+v)", sawImport, sawMAC, events)
	}
}

func TestSealedVaultIsVault(t *testing.T) {
	var _ Vault = newSealed(t) // compile-time + run-time interface satisfaction
}
