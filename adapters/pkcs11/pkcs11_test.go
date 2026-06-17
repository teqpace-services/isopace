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

package pkcs11_test

import (
	"testing"

	pkcs11 "github.com/teqpace-services/isopace/adapters/pkcs11"
	"github.com/teqpace-services/isopace/vault"
)

// The adapter advertises exactly the MAC capability.
var _ vault.Macer = (*pkcs11.Vault)(nil)

// TestCapabilitySurface pins the central security property of this adapter: a
// general-purpose PKCS#11 HSM can MAC but must NOT be mistakable for a PIN
// translator. This is exactly the check a switch performs before trusting a
// vault with PINs (see vault.PINTranslator's contract).
func TestCapabilitySurface(t *testing.T) {
	var m vault.Macer = (*pkcs11.Vault)(nil)
	if _, ok := m.(vault.PINTranslator); ok {
		t.Error("pkcs11.Vault must NOT satisfy vault.PINTranslator (no PCI-secure translate on stock PKCS#11)")
	}
	if _, ok := m.(vault.PINEncryptor); ok {
		t.Error("pkcs11.Vault must NOT satisfy vault.PINEncryptor (clear-PIN op belongs to an issuer-context adapter)")
	}
	if _, ok := m.(vault.Vault); ok {
		t.Error("pkcs11.Vault must NOT satisfy the full vault.Vault")
	}
}

func TestOpenRequiresModulePath(t *testing.T) {
	if _, err := pkcs11.Open(pkcs11.Config{}); err == nil {
		t.Fatal("Open with no ModulePath should error")
	}
}

func TestOpenBadModule(t *testing.T) {
	// A non-existent module must fail cleanly, not panic.
	if _, err := pkcs11.Open(pkcs11.Config{ModulePath: "/nonexistent/libnope.so"}); err == nil {
		t.Fatal("Open with a bogus module path should error")
	}
}
