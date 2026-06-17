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

// Package pkcs11 implements the Isopace [vault.Macer] capability against a
// PKCS#11 cryptographic token (a general-purpose HSM). Keys are referenced by
// their CKA_LABEL and the key material never leaves the token.
//
// # Capability: MAC only
//
// A general-purpose PKCS#11 HSM can compute message authentication codes but
// CANNOT perform a PCI-compliant PIN translate, so this adapter implements
// [vault.Macer] and nothing more. It deliberately does NOT implement
// [vault.PINTranslator] or [vault.PINEncryptor]:
//
//   - PIN translate must be a single atomic operation inside the device so the
//     clear PIN never reaches host memory. Standard PKCS#11 has no such
//     mechanism; emulating it with C_Decrypt → re-encode → C_Encrypt would
//     materialise the clear PIN block in the host and violate PCI PIN Security.
//     Per the [vault.PINTranslator] contract an adapter that cannot translate
//     PIN-securely MUST NOT advertise the interface — so this type omits the
//     method entirely. Use the payment-HSM adapter (e.g. payShield) for PIN
//     translation.
//   - PIN-block encryption takes a clear PIN, so it belongs to an issuer /
//     trusted-context adapter, not this stock-PKCS#11 one.
//
// # How the MAC is computed
//
// ISO 9797-1 padding and the message are not secret, so the padding is applied
// in the host and the keyed block operations run on the token; the key never
// leaves it. Modern tokens disable single-DES and expose no retail-MAC
// mechanism (CKM_DES3_MAC is a FULL-3DES CBC-MAC, a different algorithm), so the
// MAC is composed from the 3DES primitives the token does provide (CKM_DES3_CBC,
// CKM_DES3_ECB). A single-DES operation E_K is obtained by presenting K as a
// triple-length key K‖K‖K, since 3DES-EDE then collapses to E_K.
//
//   - [vault.MACAlg1] (single-DES CBC-MAC): the MAC is the final block of a
//     CKM_DES3_CBC pass under a K‖K‖K key.
//   - [vault.MACAlg3] (ANSI X9.19 retail MAC, double-length key K1‖K2): the
//     single-DES CBC-MAC under K1 over all but the last block (CKM_DES3_CBC under
//     a K1‖K1‖K1 key), then a 3DES-EDE of (lastBlock XOR prefixMAC) under the
//     natural retail key K1‖K2‖K1 (CKM_DES3_ECB). That EDE is E_K1(D_K2(E_K1(·))),
//     which folds in the retail MAC's final transform E_K1(D_K2(·)).
//
// Key provisioning (the key never leaves the token):
//
//   - MACAlg1: keyRef is a CKK_DES3 key with value K‖K‖K.
//   - MACAlg3: keyRef is the natural retail key as a CKK_DES3 value K1‖K2‖K1; and
//     keyRef+Config.RetailCBCLabelSuffix (default "-cbc") is a CKK_DES3 key with
//     value K1‖K1‖K1, used for the single-DES CBC stage.
//
// The output is the full 8-byte MAC, byte-for-byte identical to
// [vault.GenerateMAC]; callers truncate to the agreed length themselves.
//
// # Status
//
// The MAC path is functional and cross-checked against the [vault] software
// reference under SoftHSM2 in CI (see pkcs11_softhsm_test.go). It still requires
// independent security review and validation against the specific certified HSM
// (PCI PIN Security / FIPS) before production use. The approach needs the MAC key
// usable for 3DES encrypt; a hardened device that restricts MAC keys to CKA_SIGN
// with a native MAC/CMAC mechanism should use that mechanism instead (wire it in
// per device, or use the payment-HSM adapter).
package pkcs11

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"sync"

	p11 "github.com/miekg/pkcs11"

	"github.com/teqpace-services/isopace/vault"
)

// Vault implements vault.Macer. It deliberately does not implement
// vault.PINTranslator or vault.PINEncryptor (see the package doc).
var _ vault.Macer = (*Vault)(nil)

// ErrUnsupportedAlgorithm is returned for a MAC algorithm this adapter does not
// implement on stock PKCS#11.
var ErrUnsupportedAlgorithm = errors.New("pkcs11: unsupported MAC algorithm")

const desBlockSize = 8

// Config configures a PKCS#11-backed Vault.
type Config struct {
	ModulePath string // path to the PKCS#11 module, e.g. /usr/lib/softhsm/libsofthsm2.so
	TokenLabel string // label of the token to use; empty selects the first token
	PIN        string // user PIN for C_Login; empty skips login (public session)
	KeyClass   uint   // CKA_CLASS for key lookup; 0 defaults to CKO_SECRET_KEY

	// RetailCBCLabelSuffix is appended to keyRef to find the K1‖K1‖K1 helper key
	// used for the single-DES CBC stage of a retail MAC (MACAlg3); empty defaults
	// to "-cbc". keyRef itself is the natural retail key (K1‖K2‖K1).
	RetailCBCLabelSuffix string
}

// Vault is a vault.Macer backed by a PKCS#11 token. It is safe for concurrent
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

// GenerateMAC implements vault.Macer. It returns the full 8-byte ISO 9797-1 MAC,
// computed on the token; the key never leaves the device. See the package doc
// for the algorithm → mechanism mapping and the retail-key labelling.
func (v *Vault) GenerateMAC(keyRef string, alg vault.MACAlgorithm, pad vault.Padding, data []byte) ([]byte, error) {
	padded := iso9797Pad(pad, data)

	v.mu.Lock()
	defer v.mu.Unlock()

	switch alg {
	case vault.MACAlg1:
		k, err := v.findKey(keyRef)
		if err != nil {
			return nil, err
		}
		return v.cbcMACLastBlock(k, padded)

	case vault.MACAlg3:
		// Retail MAC = E_K1(D_K2(H_n)) where H_n is the single-DES CBC-MAC under
		// K1. Using H_n = E_K1(lastBlock XOR H_{n-1}), this equals a 3DES-EDE
		// E_K1(D_K2(E_K1(·))) of (lastBlock XOR H_{n-1}) under the natural retail
		// key K1‖K2‖K1, where H_{n-1} is the CBC-MAC of all but the last block.
		final, err := v.findKey(keyRef)
		if err != nil {
			return nil, err
		}
		prefixKey, err := v.findKey(keyRef + v.retailCBCSuffix())
		if err != nil {
			return nil, fmt.Errorf("pkcs11: retail-MAC CBC key (single-DES under K1): %w", err)
		}
		n := len(padded) / desBlockSize
		prefixMAC := make([]byte, desBlockSize) // zero when only one block
		if n > 1 {
			prefixMAC, err = v.cbcMACLastBlock(prefixKey, padded[:desBlockSize*(n-1)])
			if err != nil {
				return nil, err
			}
		}
		x := xorBlock(padded[desBlockSize*(n-1):], prefixMAC)
		return v.ecbEncrypt(final, x)

	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedAlgorithm, alg)
	}
}

// VerifyMAC implements vault.Macer. It recomputes the MAC and constant-time
// compares its leftmost len(mac) bytes to mac (matching vault.VerifyMAC).
func (v *Vault) VerifyMAC(keyRef string, alg vault.MACAlgorithm, pad vault.Padding, data, mac []byte) (bool, error) {
	full, err := v.GenerateMAC(keyRef, alg, pad, data)
	if err != nil {
		return false, err
	}
	if len(mac) == 0 || len(mac) > len(full) {
		return false, fmt.Errorf("pkcs11: MAC length %d out of range", len(mac))
	}
	return subtle.ConstantTimeCompare(full[:len(mac)], mac) == 1, nil
}

func (v *Vault) retailCBCSuffix() string {
	if v.cfg.RetailCBCLabelSuffix == "" {
		return "-cbc"
	}
	return v.cfg.RetailCBCLabelSuffix
}

// cbcMACLastBlock returns the final cipher block of a zero-IV CKM_DES3_CBC
// encryption of padded (a whole number of 8-byte blocks) under the key handle.
// With a K‖K‖K key this is the single-DES CBC-MAC under K. The caller must hold
// v.mu.
func (v *Vault) cbcMACLastBlock(key p11.ObjectHandle, padded []byte) ([]byte, error) {
	iv := make([]byte, desBlockSize)
	m := []*p11.Mechanism{p11.NewMechanism(p11.CKM_DES3_CBC, iv)}
	if err := v.ctx.EncryptInit(v.session, m, key); err != nil {
		return nil, fmt.Errorf("pkcs11: C_EncryptInit(DES3-CBC): %w", err)
	}
	ct, err := v.ctx.Encrypt(v.session, padded)
	if err != nil {
		return nil, fmt.Errorf("pkcs11: C_Encrypt(DES3-CBC): %w", err)
	}
	if len(ct) < desBlockSize {
		return nil, fmt.Errorf("pkcs11: short DES3-CBC output (%d bytes)", len(ct))
	}
	return ct[len(ct)-desBlockSize:], nil
}

// ecbEncrypt runs one CKM_DES3_ECB block encryption under the key handle. With a
// K1‖K2‖K1 key this is the 3DES-EDE E_K1(D_K2(E_K1(·))). The caller must hold
// v.mu.
func (v *Vault) ecbEncrypt(key p11.ObjectHandle, block []byte) ([]byte, error) {
	m := []*p11.Mechanism{p11.NewMechanism(p11.CKM_DES3_ECB, nil)}
	if err := v.ctx.EncryptInit(v.session, m, key); err != nil {
		return nil, fmt.Errorf("pkcs11: C_EncryptInit(DES3-ECB): %w", err)
	}
	out, err := v.ctx.Encrypt(v.session, block)
	if err != nil {
		return nil, fmt.Errorf("pkcs11: C_Encrypt(DES3-ECB): %w", err)
	}
	return out, nil
}

// xorBlock returns a XOR b over the leftmost 8 bytes (the DES block size).
func xorBlock(a, b []byte) []byte {
	out := make([]byte, desBlockSize)
	for i := range desBlockSize {
		out[i] = a[i] ^ b[i]
	}
	return out
}

// iso9797Pad applies ISO 9797-1 padding to a multiple of the 8-byte DES block.
// It mirrors the (unexported) padding in vault.GenerateMAC; the SoftHSM
// functional test cross-checks the resulting MAC against vault.GenerateMAC, so
// any divergence here would fail CI.
//
//   - Pad1 (method 1): minimum zero bytes to fill the last block (one zero block
//     for empty input).
//   - Pad2 (method 2): a 0x80 byte then zero bytes to fill the last block.
func iso9797Pad(pad vault.Padding, data []byte) []byte {
	switch pad {
	case vault.Pad2:
		out := make([]byte, 0, len(data)+desBlockSize)
		out = append(out, data...)
		out = append(out, 0x80)
		for len(out)%desBlockSize != 0 {
			out = append(out, 0x00)
		}
		return out
	default: // Pad1
		if len(data) == 0 {
			return make([]byte, desBlockSize)
		}
		out := append([]byte(nil), data...)
		for len(out)%desBlockSize != 0 {
			out = append(out, 0x00)
		}
		return out
	}
}
