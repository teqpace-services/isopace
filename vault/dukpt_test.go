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
	"bytes"
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// The canonical ANSI X9.24-1 TDES DUKPT test vector, widely published.
func TestDUKPTKnownVector(t *testing.T) {
	bdk := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	ksn := mustHex(t, "FFFF9876543210E00000")

	ipek, err := DeriveIPEK(bdk, ksn)
	if err != nil {
		t.Fatalf("DeriveIPEK: %v", err)
	}
	wantIPEK := mustHex(t, "6AC292FAA1315B4D858AB3A3D7D5933A")
	if !bytes.Equal(ipek, wantIPEK) {
		t.Fatalf("IPEK = %X want %X", ipek, wantIPEK)
	}

	// First transaction (counter = 1): the derived transaction key is used
	// directly for PIN encryption in classic DUKPT, and equals the published
	// vector value.
	ksn1 := mustHex(t, "FFFF9876543210E00001")
	sk, err := DeriveSessionKey(ipek, ksn1)
	if err != nil {
		t.Fatalf("DeriveSessionKey: %v", err)
	}
	wantKey := mustHex(t, "042666B49184CFA368DE9628D0397BC9")
	if !bytes.Equal(sk, wantKey) {
		t.Fatalf("transaction key (KSN ...0001) = %X want %X", sk, wantKey)
	}

	// The 2009 variant scheme just XORs the constant.
	pek, err := DeriveVariant(sk, VariantPIN)
	if err != nil {
		t.Fatalf("DeriveVariant: %v", err)
	}
	if bytes.Equal(pek, sk) {
		t.Errorf("PIN variant did not change the key")
	}

	// Counter must advance the key: a different KSN yields a different key.
	ksn2 := mustHex(t, "FFFF9876543210E00002")
	sk2, err := DeriveSessionKey(ipek, ksn2)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(sk, sk2) {
		t.Errorf("counters 1 and 2 derived the same key")
	}
	if c, _ := KSNCounter(ksn2); c != 2 {
		t.Errorf("KSNCounter = %d want 2", c)
	}
}
