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

package vault_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/teqpace-services/isopace/vault"
)

func TestSHA256MAC(t *testing.T) {
	key := []byte{0x0A, 0x1B, 0x2C, 0x3D, 0x4E, 0x5F, 0x60, 0x71}
	data := []byte("0200ABCDEF0123456789")

	got := vault.SHA256MAC(key, data)

	// Contract: SHA-256 over key THEN data (prefix-keyed, not HMAC).
	want := sha256.Sum256(append(append([]byte{}, key...), data...))
	if hex.EncodeToString(got) != hex.EncodeToString(want[:]) {
		t.Fatalf("SHA256MAC = %x, want %x", got, want)
	}
	if len(hex.EncodeToString(got)) != 64 {
		t.Fatalf("hex MAC length = %d, want 64", len(hex.EncodeToString(got)))
	}

	ok, err := vault.VerifySHA256MAC(key, data, got)
	if err != nil || !ok {
		t.Fatalf("VerifySHA256MAC(correct) = %v, %v; want true, nil", ok, err)
	}
	ok, _ = vault.VerifySHA256MAC(key, append(data, 'x'), got)
	if ok {
		t.Fatal("VerifySHA256MAC accepted a MAC over tampered data")
	}
	if _, err := vault.VerifySHA256MAC(key, data, nil); err == nil {
		t.Fatal("VerifySHA256MAC accepted an empty MAC")
	}
}
