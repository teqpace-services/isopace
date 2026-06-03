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

package pkcs11_test

import (
	"errors"
	"testing"

	pkcs11 "github.com/teqpace-services/isopace/adapters/pkcs11"
	"github.com/teqpace-services/isopace/vault"
)

// The adapter must satisfy the core Vault interface at compile time.
var _ vault.Vault = (*pkcs11.Vault)(nil)

// The cryptographic methods are stubs for now; they must report that clearly and
// must not panic (they do not touch the session). A functional suite against
// SoftHSM is added once the operations are implemented.
func TestCryptoMethodsStubbed(t *testing.T) {
	var v pkcs11.Vault

	if _, err := v.EncryptPINBlock("k", vault.ISO0, "1234", "4111111111111111"); !errors.Is(err, pkcs11.ErrNotImplemented) {
		t.Errorf("EncryptPINBlock err = %v, want ErrNotImplemented", err)
	}
	if _, err := v.TranslatePIN("s", "d", []byte{0}, "4111111111111111", vault.ISO0, vault.ISO0); !errors.Is(err, pkcs11.ErrNotImplemented) {
		t.Errorf("TranslatePIN err = %v, want ErrNotImplemented", err)
	}
	if _, err := v.GenerateMAC("k", vault.MACAlg1, vault.Pad1, []byte("x")); !errors.Is(err, pkcs11.ErrNotImplemented) {
		t.Errorf("GenerateMAC err = %v, want ErrNotImplemented", err)
	}
	if ok, err := v.VerifyMAC("k", vault.MACAlg1, vault.Pad1, []byte("x"), []byte("y")); ok || !errors.Is(err, pkcs11.ErrNotImplemented) {
		t.Errorf("VerifyMAC = %v, %v, want false, ErrNotImplemented", ok, err)
	}
}

func TestOpenRequiresModulePath(t *testing.T) {
	if _, err := pkcs11.Open(pkcs11.Config{}); err == nil {
		t.Fatal("Open with no ModulePath should error")
	}
}
