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

package vault_test

import (
	"testing"

	"github.com/teqpace-services/isopace/vault"
)

// The software vault satisfies every capability interface and the full Vault,
// verified from an external package (as an adapter would see them).
var (
	_ vault.PINEncryptor  = (*vault.SoftVault)(nil)
	_ vault.PINTranslator = (*vault.SoftVault)(nil)
	_ vault.Macer         = (*vault.SoftVault)(nil)
	_ vault.Vault         = (*vault.SoftVault)(nil)
)

// macOnly stands in for a general-purpose (e.g. PKCS#11) HSM adapter that can
// MAC but cannot translate PINs.
type macOnly struct{}

func (macOnly) GenerateMAC(string, vault.MACAlgorithm, vault.Padding, []byte) ([]byte, error) {
	return nil, nil
}

func (macOnly) VerifyMAC(string, vault.MACAlgorithm, vault.Padding, []byte, []byte) (bool, error) {
	return false, nil
}

func TestCapabilityDetection(t *testing.T) {
	// A full software vault advertises every capability.
	var v vault.Vault = vault.NewSoftVault()
	if _, ok := v.(vault.PINTranslator); !ok {
		t.Error("SoftVault should satisfy PINTranslator")
	}
	if _, ok := v.(vault.Macer); !ok {
		t.Error("SoftVault should satisfy Macer")
	}

	// A MAC-only adapter must NOT be mistakable for a PIN translator — this is
	// exactly the check a switch performs before trusting a vault with PINs.
	var m vault.Macer = macOnly{}
	if _, ok := m.(vault.PINTranslator); ok {
		t.Error("a Macer-only vault must not satisfy PINTranslator")
	}
}
