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

// Package pkcs11 implements the Isopace vault.Vault façade against a PKCS#11
// cryptographic token (an HSM). Keys are referenced by their CKA_LABEL and the
// key material never leaves the token.
//
// # Status
//
// This module provides the connection/session/key-lookup FOUNDATION, which is
// build-verified. The four cryptographic Vault methods are deliberately STUBBED
// (they return ErrNotImplemented) — they are not yet implemented because:
//
//   - GenerateMAC/VerifyMAC require an ISO 9797-1 → PKCS#11 mechanism mapping
//     that is HSM-specific and must be verified against a real token (SoftHSM in
//     CI) and security-reviewed before use; and
//   - TranslatePIN cannot be implemented securely on stock PKCS#11. The
//     vault.Vault contract (decrypt under src, re-encode, re-encrypt under dst)
//     would expose the clear PIN block in host memory, which violates PCI PIN
//     Security. A secure translate is a single atomic HSM operation, which is a
//     vendor-specific mechanism, not part of standard PKCS#11. This needs a
//     vault-API decision (see README and ROADMAP-to-v1.md, B1).
//
// Nothing here is production-ready: it requires independent security review and
// validation against a certified HSM (PCI PIN Security / FIPS) before use.
package pkcs11

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	p11 "github.com/miekg/pkcs11"

	"github.com/teqpace-services/isopace/vault"
)

// Vault implements vault.Vault (the crypto methods are stubbed; see package doc).
var _ vault.Vault = (*Vault)(nil)

// ErrNotImplemented marks a cryptographic operation whose secure PKCS#11
// implementation is pending verification and security review.
var ErrNotImplemented = errors.New("pkcs11: cryptographic operation not yet implemented (foundation only)")

// Config configures a PKCS#11-backed Vault.
type Config struct {
	ModulePath string // path to the PKCS#11 module, e.g. /usr/lib/softhsm/libsofthsm2.so
	TokenLabel string // label of the token to use; empty selects the first token
	PIN        string // user PIN for C_Login; empty skips login (public session)
	KeyClass   uint   // CKA_CLASS for key lookup; 0 defaults to CKO_SECRET_KEY
}

// Vault is a vault.Vault backed by a PKCS#11 token. It is safe for concurrent
// use: a single session is serialised by a mutex (a session pool is a future
// optimisation).
type Vault struct {
	cfg      Config
	ctx      *p11.Ctx
	mu       sync.Mutex
	session  p11.SessionHandle
	loggedIn bool
}

// Open loads the PKCS#11 module, selects the configured token, opens a session,
// and logs in (when a PIN is provided). Call Close to release resources.
func Open(cfg Config) (*Vault, error) {
	if cfg.ModulePath == "" {
		return nil, errors.New("pkcs11: Config.ModulePath is required")
	}
	ctx := p11.New(cfg.ModulePath)
	if ctx == nil {
		return nil, fmt.Errorf("pkcs11: could not load module %q", cfg.ModulePath)
	}
	if err := ctx.Initialize(); err != nil {
		ctx.Destroy()
		return nil, fmt.Errorf("pkcs11: C_Initialize: %w", err)
	}
	v := &Vault{cfg: cfg, ctx: ctx}

	slot, err := v.findSlot()
	if err != nil {
		v.teardown()
		return nil, err
	}
	sess, err := ctx.OpenSession(slot, p11.CKF_SERIAL_SESSION)
	if err != nil {
		v.teardown()
		return nil, fmt.Errorf("pkcs11: C_OpenSession: %w", err)
	}
	v.session = sess
	if cfg.PIN != "" {
		if err := ctx.Login(sess, p11.CKU_USER, cfg.PIN); err != nil {
			_ = ctx.CloseSession(sess)
			v.teardown()
			return nil, fmt.Errorf("pkcs11: C_Login: %w", err)
		}
		v.loggedIn = true
	}
	return v, nil
}

// findSlot returns the slot whose token label matches Config.TokenLabel (or the
// first token-present slot when the label is empty).
func (v *Vault) findSlot() (uint, error) {
	slots, err := v.ctx.GetSlotList(true)
	if err != nil {
		return 0, fmt.Errorf("pkcs11: C_GetSlotList: %w", err)
	}
	for _, s := range slots {
		ti, err := v.ctx.GetTokenInfo(s)
		if err != nil {
			continue
		}
		if v.cfg.TokenLabel == "" || strings.TrimSpace(ti.Label) == v.cfg.TokenLabel {
			return s, nil
		}
	}
	return 0, fmt.Errorf("pkcs11: no token found (label %q)", v.cfg.TokenLabel)
}

func (v *Vault) teardown() {
	_ = v.ctx.Finalize()
	v.ctx.Destroy()
}

// Close logs out (if logged in), closes the session, and unloads the module.
func (v *Vault) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.loggedIn {
		_ = v.ctx.Logout(v.session)
		v.loggedIn = false
	}
	_ = v.ctx.CloseSession(v.session)
	v.teardown()
	return nil
}

// findKey resolves a key reference (matched against CKA_LABEL) to an object
// handle within the active session. The caller must hold v.mu.
func (v *Vault) findKey(ref string) (p11.ObjectHandle, error) {
	class := v.cfg.KeyClass
	if class == 0 {
		class = p11.CKO_SECRET_KEY
	}
	template := []*p11.Attribute{
		p11.NewAttribute(p11.CKA_CLASS, class),
		p11.NewAttribute(p11.CKA_LABEL, ref),
	}
	if err := v.ctx.FindObjectsInit(v.session, template); err != nil {
		return 0, fmt.Errorf("pkcs11: C_FindObjectsInit: %w", err)
	}
	objs, _, err := v.ctx.FindObjects(v.session, 1)
	finalErr := v.ctx.FindObjectsFinal(v.session)
	if err != nil {
		return 0, fmt.Errorf("pkcs11: C_FindObjects: %w", err)
	}
	if finalErr != nil {
		return 0, fmt.Errorf("pkcs11: C_FindObjectsFinal: %w", finalErr)
	}
	if len(objs) == 0 {
		return 0, fmt.Errorf("%w: %q", vault.ErrUnknownKey, ref)
	}
	return objs[0], nil
}

// EncryptPINBlock implements vault.Vault.
//
// Intended PKCS#11 mapping: encode the clear block with vault.EncodePINBlock,
// then C_Encrypt the 8-byte block under the CKM_DES3_ECB mechanism with the key
// handle resolved from keyRef.
//
// NOTE: this still forms the clear PIN block in host memory (the interface takes
// a clear `pin string`). See the package and README security notes. Stubbed
// pending verification against a real token and security review.
func (v *Vault) EncryptPINBlock(keyRef string, format vault.PINBlockFormat, pin, pan string) ([]byte, error) {
	return nil, ErrNotImplemented
}

// TranslatePIN implements vault.Vault.
//
// A secure HSM PIN-translate is a single atomic operation in which the clear PIN
// never leaves the token. Standard PKCS#11 has no such mechanism; emulating the
// vault.Vault contract via C_Decrypt → re-encode → C_Encrypt would expose the
// clear PIN block in host memory and violate PCI PIN Security. Left unimplemented
// pending a vault-API decision (see README and ROADMAP-to-v1.md, B1).
func (v *Vault) TranslatePIN(srcRef, dstRef string, encBlock []byte, pan string, srcFormat, dstFormat vault.PINBlockFormat) ([]byte, error) {
	return nil, fmt.Errorf("%w: secure PIN translate needs an atomic HSM mechanism, not stock PKCS#11", ErrNotImplemented)
}

// GenerateMAC implements vault.Vault.
//
// Intended PKCS#11 mapping: C_SignInit/C_Sign under a mechanism chosen from
// (alg, pad) — e.g. CKM_DES3_MAC / CKM_DES3_MAC_GENERAL for ISO 9797-1, or
// CKM_AES_CMAC for CMAC. The exact mapping is HSM-specific and must be verified
// against a real token (SoftHSM in CI) and security-reviewed. Stubbed for now.
func (v *Vault) GenerateMAC(keyRef string, alg vault.MACAlgorithm, pad vault.Padding, data []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}

// VerifyMAC implements vault.Vault.
//
// Intended PKCS#11 mapping: C_VerifyInit/C_Verify under the same mechanism as
// GenerateMAC, or recompute and compare in constant time. Stubbed for now.
func (v *Vault) VerifyMAC(keyRef string, alg vault.MACAlgorithm, pad vault.Padding, data, mac []byte) (bool, error) {
	return false, ErrNotImplemented
}
