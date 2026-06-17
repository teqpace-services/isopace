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
	"bytes"
	"os"
	"testing"

	p11 "github.com/miekg/pkcs11"

	pkcs11 "github.com/teqpace-services/isopace/adapters/pkcs11"
	"github.com/teqpace-services/isopace/vault"
)

// This functional suite runs only when a PKCS#11 token is configured via the
// environment (CI provisions SoftHSM2). It proves the adapter drives a real
// token to produce a MAC that is byte-for-byte identical to the vault software
// reference — so the HSM path is interoperable, not merely self-consistent.
//
//	ISOPACE_SOFTHSM_MODULE  path to the PKCS#11 module (required to run)
//	ISOPACE_SOFTHSM_TOKEN   token label   (default "isopace")
//	ISOPACE_SOFTHSM_PIN     user PIN      (default "1234")

const (
	macKeyLabel    = "isopace-mac1"     // CKK_DES3 K‖K‖K        (MACAlg1)
	retailKeyLabel = "isopace-macr"     // CKK_DES3 K1‖K2‖K1     (MACAlg3 final EDE)
	retailCBCLabel = "isopace-macr-cbc" // CKK_DES3 K1‖K1‖K1     (MACAlg3 CBC stage)
)

// Known test key halves (odd parity so a parity-checking token accepts them; DES
// ignores the parity bit, so vault and the token compute the same cipher). vault
// MACs with the natural 8-/16-byte keys; the token holds the triple-length
// expansions (single-DES E_K presented as 3DES K‖K‖K) — and the MACs must match.
var (
	desKey  = oddParity([]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF})
	retailL = oddParity([]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88})
	retailR = oddParity([]byte{0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00})
)

// cat concatenates 8-byte halves into a triple-length (24-byte) DES3 key value.
func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func softhsmConfig(t *testing.T) pkcs11.Config {
	t.Helper()
	mod := os.Getenv("ISOPACE_SOFTHSM_MODULE")
	if mod == "" {
		t.Skip("ISOPACE_SOFTHSM_MODULE not set; skipping SoftHSM functional suite")
	}
	token := os.Getenv("ISOPACE_SOFTHSM_TOKEN")
	if token == "" {
		token = "isopace"
	}
	pin := os.Getenv("ISOPACE_SOFTHSM_PIN")
	if pin == "" {
		pin = "1234"
	}
	return pkcs11.Config{ModulePath: mod, TokenLabel: token, PIN: pin}
}

func TestSoftHSM_MAC(t *testing.T) {
	cfg := softhsmConfig(t)

	// Provision the test keys with known values via an independent session, then
	// exercise them through the adapter. Cleanup destroys them afterwards.
	cleanup := provisionKeys(t, cfg)
	defer cleanup()

	v, err := pkcs11.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	retailKey := append(append([]byte(nil), retailL...), retailR...)

	cases := []struct {
		name   string
		keyRef string
		alg    vault.MACAlgorithm
		key    []byte
	}{
		{"alg1 single-DES", macKeyLabel, vault.MACAlg1, desKey},
		{"alg3 retail", retailKeyLabel, vault.MACAlg3, retailKey},
	}
	pads := []struct {
		name string
		pad  vault.Padding
	}{{"pad1", vault.Pad1}, {"pad2", vault.Pad2}}
	msgs := [][]byte{
		nil,
		[]byte("8"),
		[]byte("hello"),
		[]byte("0123456789ABCDEF"), // two whole blocks
		[]byte("an ISO 8583 0200 request payload of some length"),
	}

	for _, c := range cases {
		for _, p := range pads {
			for i, msg := range msgs {
				t.Run(c.name+"/"+p.name+"/"+msgName(i), func(t *testing.T) {
					want, err := vault.GenerateMAC(c.alg, p.pad, c.key, msg)
					if err != nil {
						t.Fatalf("vault.GenerateMAC: %v", err)
					}
					got, err := v.GenerateMAC(c.keyRef, c.alg, p.pad, msg)
					if err != nil {
						t.Fatalf("adapter.GenerateMAC: %v", err)
					}
					if !bytes.Equal(got, want) {
						t.Fatalf("MAC mismatch\n adapter = % x\n vault   = % x", got, want)
					}

					// Verify: full match, truncated (4-byte) match, and tamper.
					if ok, err := v.VerifyMAC(c.keyRef, c.alg, p.pad, msg, want); err != nil || !ok {
						t.Fatalf("VerifyMAC(full) = %v, %v; want true, nil", ok, err)
					}
					if ok, err := v.VerifyMAC(c.keyRef, c.alg, p.pad, msg, want[:4]); err != nil || !ok {
						t.Fatalf("VerifyMAC(4-byte) = %v, %v; want true, nil", ok, err)
					}
					bad := append([]byte(nil), want...)
					bad[0] ^= 0xFF
					if ok, _ := v.VerifyMAC(c.keyRef, c.alg, p.pad, msg, bad); ok {
						t.Fatal("VerifyMAC accepted a tampered MAC")
					}
				})
			}
		}
	}
}

func TestSoftHSM_UnknownKey(t *testing.T) {
	cfg := softhsmConfig(t)
	v, err := pkcs11.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()
	if _, err := v.GenerateMAC("isopace-no-such-key", vault.MACAlg1, vault.Pad1, []byte("x")); err == nil {
		t.Fatal("GenerateMAC with an unknown key should error")
	}
}

func msgName(i int) string { return []string{"empty", "1byte", "5byte", "2block", "long"}[i] }

// provisionKeys creates the test DES keys as persistent token objects, then
// returns a cleanup that destroys them. Each phase opens and FULLY finalizes its
// own PKCS#11 context: C_Initialize is process-global, so the provisioning
// context must not overlap the adapter's. The keys survive between phases
// because they are token (persistent) objects.
func provisionKeys(t *testing.T, cfg pkcs11.Config) func() {
	t.Helper()
	withProvisionSession(t, cfg, func(ctx *p11.Ctx, sess p11.SessionHandle) {
		// Remove any leftovers from a previous interrupted run, then (re)create
		// the triple-length expansions the adapter expects on the token.
		destroyByLabel(ctx, sess, macKeyLabel)
		destroyByLabel(ctx, sess, retailKeyLabel)
		destroyByLabel(ctx, sess, retailCBCLabel)
		createDES3Key(t, ctx, sess, macKeyLabel, cat(desKey, desKey, desKey))
		createDES3Key(t, ctx, sess, retailKeyLabel, cat(retailL, retailR, retailL))
		createDES3Key(t, ctx, sess, retailCBCLabel, cat(retailL, retailL, retailL))
	})
	return func() {
		withProvisionSession(t, cfg, func(ctx *p11.Ctx, sess p11.SessionHandle) {
			destroyByLabel(ctx, sess, macKeyLabel)
			destroyByLabel(ctx, sess, retailKeyLabel)
			destroyByLabel(ctx, sess, retailCBCLabel)
		})
	}
}

// withProvisionSession opens a logged-in R/W session on the configured token,
// runs fn, and fully tears the context down (logout, close, finalize, destroy)
// before returning — so no PKCS#11 context outlives this call.
func withProvisionSession(t *testing.T, cfg pkcs11.Config, fn func(*p11.Ctx, p11.SessionHandle)) {
	t.Helper()
	ctx := p11.New(cfg.ModulePath)
	if ctx == nil {
		t.Fatalf("could not load module %q", cfg.ModulePath)
	}
	if err := ctx.Initialize(); err != nil {
		t.Fatalf("C_Initialize: %v", err)
	}
	defer ctx.Destroy()
	defer ctx.Finalize()

	slots, err := ctx.GetSlotList(true)
	if err != nil || len(slots) == 0 {
		t.Fatalf("GetSlotList: %v (slots=%d)", err, len(slots))
	}
	var slot uint
	found := false
	for _, s := range slots {
		ti, err := ctx.GetTokenInfo(s)
		if err == nil && (cfg.TokenLabel == "" || trim(ti.Label) == cfg.TokenLabel) {
			slot, found = s, true
			break
		}
	}
	if !found {
		t.Fatalf("token %q not found", cfg.TokenLabel)
	}
	sess, err := ctx.OpenSession(slot, p11.CKF_SERIAL_SESSION|p11.CKF_RW_SESSION)
	if err != nil {
		t.Fatalf("OpenSession(RW): %v", err)
	}
	defer ctx.CloseSession(sess)
	if err := ctx.Login(sess, p11.CKU_USER, cfg.PIN); err != nil {
		t.Fatalf("C_Login: %v", err)
	}
	defer ctx.Logout(sess)

	fn(ctx, sess)
}

func createDES3Key(t *testing.T, ctx *p11.Ctx, sess p11.SessionHandle, label string, value []byte) p11.ObjectHandle {
	t.Helper()
	tmpl := []*p11.Attribute{
		p11.NewAttribute(p11.CKA_CLASS, p11.CKO_SECRET_KEY),
		p11.NewAttribute(p11.CKA_KEY_TYPE, p11.CKK_DES3),
		p11.NewAttribute(p11.CKA_TOKEN, true),
		p11.NewAttribute(p11.CKA_PRIVATE, true),
		p11.NewAttribute(p11.CKA_LABEL, label),
		p11.NewAttribute(p11.CKA_ENCRYPT, true),
		p11.NewAttribute(p11.CKA_DECRYPT, true),
		p11.NewAttribute(p11.CKA_VALUE, value),
	}
	h, err := ctx.CreateObject(sess, tmpl)
	if err != nil {
		t.Fatalf("C_CreateObject(%s): %v", label, err)
	}
	return h
}

func destroyByLabel(ctx *p11.Ctx, sess p11.SessionHandle, label string) {
	tmpl := []*p11.Attribute{
		p11.NewAttribute(p11.CKA_CLASS, p11.CKO_SECRET_KEY),
		p11.NewAttribute(p11.CKA_LABEL, label),
	}
	if ctx.FindObjectsInit(sess, tmpl) != nil {
		return
	}
	objs, _, _ := ctx.FindObjects(sess, 16)
	_ = ctx.FindObjectsFinal(sess)
	for _, o := range objs {
		_ = ctx.DestroyObject(sess, o)
	}
}

func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == 0) {
		s = s[:len(s)-1]
	}
	return s
}

// oddParity sets the low bit of each byte to make the byte's popcount odd, the
// DES key-parity convention some tokens enforce on import.
func oddParity(k []byte) []byte {
	out := append([]byte(nil), k...)
	for i, b := range out {
		ones := 0
		for j := 1; j < 8; j++ {
			if b&(1<<uint(j)) != 0 {
				ones++
			}
		}
		if ones%2 == 0 {
			out[i] = b | 1
		} else {
			out[i] = b &^ 1
		}
	}
	return out
}
