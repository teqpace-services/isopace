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

package payshield_test

import (
	"bytes"
	"errors"
	"testing"

	payshield "github.com/teqpace-services/isopace/adapters/payshield"
	"github.com/teqpace-services/isopace/vault"
)

// A payment HSM performs every key operation in the device, so the adapter is a
// full vault.Vault.
var (
	_ vault.PINEncryptor  = (*payshield.Vault)(nil)
	_ vault.PINTranslator = (*payshield.Vault)(nil)
	_ vault.Macer         = (*payshield.Vault)(nil)
	_ vault.Vault         = (*payshield.Vault)(nil)
)

const pan = "4111111111111111"

// test keys (mirrored into the simulator and a local SoftVault for ground truth)
var (
	zpkSrc = []byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02}
	zpkDst = []byte{0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04}
	zak1   = []byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17}
	zak3   = []byte{0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2A, 0x2B, 0x2C, 0x2D, 0x2E, 0x2F}
)

// harness starts a simulator with the test keys and returns a connected adapter
// plus a local SoftVault holding the same keys (for computing expected values).
func harness(t *testing.T) (*payshield.Vault, *vault.SoftVault) {
	t.Helper()
	sim, err := payshield.NewSimulator(payshield.Config{})
	if err != nil {
		t.Fatalf("NewSimulator: %v", err)
	}
	t.Cleanup(func() { sim.Close() })

	local := vault.NewSoftVault()
	for ref, key := range map[string][]byte{
		"zpk-src": zpkSrc, "zpk-dst": zpkDst, "zak1": zak1, "zak3": zak3,
	} {
		sim.ImportKey(ref, key)
		local.SetKey(ref, key)
	}

	v, err := payshield.Open(payshield.Config{Addr: sim.Addr()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { v.Close() })
	return v, local
}

func TestCapabilitySurface(t *testing.T) {
	var pt vault.PINTranslator = (*payshield.Vault)(nil)
	if _, ok := pt.(vault.Macer); !ok {
		t.Error("payShield Vault should also satisfy vault.Macer")
	}
	if _, ok := pt.(vault.PINEncryptor); !ok {
		t.Error("payShield Vault should satisfy vault.PINEncryptor")
	}
	if _, ok := pt.(vault.Vault); !ok {
		t.Error("payShield Vault should satisfy the full vault.Vault")
	}
}

// EncryptPINBlock with ISO0 is deterministic (no random padding), so the device
// result must equal an independent software encrypt byte-for-byte. Encrypting
// then translating ISO0→ISO0 under another key must also match a local translate,
// proving the encrypted block the adapter produces is well-formed.
func TestEncryptPINBlock(t *testing.T) {
	v, local := harness(t)

	got, err := v.EncryptPINBlock("zpk-src", vault.ISO0, "1234", pan)
	if err != nil {
		t.Fatalf("EncryptPINBlock: %v", err)
	}
	want, err := local.EncryptPINBlock("zpk-src", vault.ISO0, "1234", pan)
	if err != nil {
		t.Fatalf("local EncryptPINBlock: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("encrypted PIN block\n got  = % x\n want = % x", got, want)
	}

	translated, err := v.TranslatePIN("zpk-src", "zpk-dst", got, pan, vault.ISO0, vault.ISO0)
	if err != nil {
		t.Fatalf("TranslatePIN: %v", err)
	}
	wantT, err := local.TranslatePIN("zpk-src", "zpk-dst", want, pan, vault.ISO0, vault.ISO0)
	if err != nil {
		t.Fatalf("local TranslatePIN: %v", err)
	}
	if !bytes.Equal(translated, wantT) {
		t.Fatalf("translate of adapter-encrypted block\n got  = % x\n want = % x", translated, wantT)
	}
}

// TranslatePIN with no reformat (ISO0→ISO0) is deterministic, so it must equal an
// independent software translate byte-for-byte.
func TestTranslatePIN_Deterministic(t *testing.T) {
	v, local := harness(t)

	block0, err := local.EncryptPINBlock("zpk-src", vault.ISO0, "1234", pan)
	if err != nil {
		t.Fatalf("EncryptPINBlock: %v", err)
	}
	got, err := v.TranslatePIN("zpk-src", "zpk-dst", block0, pan, vault.ISO0, vault.ISO0)
	if err != nil {
		t.Fatalf("TranslatePIN: %v", err)
	}
	want, err := local.TranslatePIN("zpk-src", "zpk-dst", block0, pan, vault.ISO0, vault.ISO0)
	if err != nil {
		t.Fatalf("local TranslatePIN: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("translated block\n got  = % x\n want = % x", got, want)
	}
}

// Reformat ISO0→ISO3 then back ISO3→ISO0 must reproduce the original (deterministic)
// ISO0 block — proving the PIN and PAN survive the reformat through the device.
func TestTranslatePIN_ReformatRoundTrip(t *testing.T) {
	v, local := harness(t)

	block0, err := local.EncryptPINBlock("zpk-src", vault.ISO0, "987654", pan)
	if err != nil {
		t.Fatalf("EncryptPINBlock: %v", err)
	}
	block3, err := v.TranslatePIN("zpk-src", "zpk-dst", block0, pan, vault.ISO0, vault.ISO3)
	if err != nil {
		t.Fatalf("TranslatePIN ISO0->ISO3: %v", err)
	}
	back0, err := v.TranslatePIN("zpk-dst", "zpk-src", block3, pan, vault.ISO3, vault.ISO0)
	if err != nil {
		t.Fatalf("TranslatePIN ISO3->ISO0: %v", err)
	}
	if !bytes.Equal(back0, block0) {
		t.Fatalf("round-trip block mismatch\n back = % x\n orig = % x", back0, block0)
	}
}

func TestMAC(t *testing.T) {
	v, local := harness(t)

	cases := []struct {
		name   string
		keyRef string
		alg    vault.MACAlgorithm
	}{
		{"alg1", "zak1", vault.MACAlg1},
		{"alg3-retail", "zak3", vault.MACAlg3},
	}
	pads := []struct {
		name string
		pad  vault.Padding
	}{{"pad1", vault.Pad1}, {"pad2", vault.Pad2}}
	msgs := [][]byte{nil, []byte("8"), []byte("hello world"), []byte("0123456789ABCDEF0123")}

	for _, c := range cases {
		for _, p := range pads {
			for _, msg := range msgs {
				t.Run(c.name+"/"+p.name, func(t *testing.T) {
					want, err := local.GenerateMAC(c.keyRef, c.alg, p.pad, msg)
					if err != nil {
						t.Fatalf("local GenerateMAC: %v", err)
					}
					got, err := v.GenerateMAC(c.keyRef, c.alg, p.pad, msg)
					if err != nil {
						t.Fatalf("GenerateMAC: %v", err)
					}
					if !bytes.Equal(got, want) {
						t.Fatalf("MAC\n got  = % x\n want = % x", got, want)
					}
					if ok, err := v.VerifyMAC(c.keyRef, c.alg, p.pad, msg, want); err != nil || !ok {
						t.Fatalf("VerifyMAC(full) = %v, %v; want true, nil", ok, err)
					}
					if ok, err := v.VerifyMAC(c.keyRef, c.alg, p.pad, msg, want[:4]); err != nil || !ok {
						t.Fatalf("VerifyMAC(4-byte) = %v, %v; want true, nil", ok, err)
					}
					bad := append([]byte(nil), want...)
					bad[0] ^= 0xFF
					if ok, err := v.VerifyMAC(c.keyRef, c.alg, p.pad, msg, bad); err != nil || ok {
						t.Fatalf("VerifyMAC(tampered) = %v, %v; want false, nil", ok, err)
					}
				})
			}
		}
	}
}

func TestUnknownKeyIsHostError(t *testing.T) {
	v, _ := harness(t)
	_, err := v.GenerateMAC("no-such-key", vault.MACAlg1, vault.Pad1, []byte("x"))
	var he *payshield.HostError
	if !errors.As(err, &he) {
		t.Fatalf("err = %v, want *payshield.HostError", err)
	}
	if he.Code != "10" {
		t.Errorf("host error code = %q, want %q (key not found)", he.Code, "10")
	}
}

func TestUnsupportedAlgorithm(t *testing.T) {
	v, _ := harness(t)
	if _, err := v.GenerateMAC("zak1", vault.MACAlgorithm(99), vault.Pad1, []byte("x")); err == nil {
		t.Fatal("GenerateMAC with an unsupported algorithm should error")
	}
}

func TestOpenUnreachable(t *testing.T) {
	if _, err := payshield.Open(payshield.Config{Addr: "127.0.0.1:1"}); err == nil {
		t.Fatal("Open against an unreachable address should error")
	}
}
