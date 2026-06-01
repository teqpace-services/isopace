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
	"testing"
)

// constReader fills with a fixed byte, for deterministic random padding in tests.
type constReader byte

func (c constReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(c)
	}
	return len(p), nil
}

func TestPINBlockISO0KnownAnswer(t *testing.T) {
	// Hand-derived ISO-0: PIN 1234, PAN 4012345678909.
	// pinField 0412 34FF FFFF FFFF ⊕ panField 0000 4012 3456 7890 = 0412 74ED CBA9 876F.
	block, err := EncodePINBlock(ISO0, "1234", "4012345678909")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := mustHex(t, "041274EDCBA9876F")
	if !bytes.Equal(block, want) {
		t.Fatalf("ISO-0 block = %X want %X", block, want)
	}
	pin, err := DecodePINBlock(ISO0, block, "4012345678909")
	if err != nil || pin != "1234" {
		t.Errorf("Decode = %q,%v want 1234", pin, err)
	}
}

func TestPINBlockRoundTrip(t *testing.T) {
	pan := "4012345678909"
	for _, tc := range []struct {
		format PINBlockFormat
		pin    string
	}{
		{ISO0, "1234"},
		{ISO0, "123456789012"},
		{ISO1, "4321"},
		{ISO3, "987654"},
	} {
		block, err := EncodePINBlockRand(tc.format, tc.pin, pan, constReader(0xAB))
		if err != nil {
			t.Fatalf("Encode(%d,%s): %v", tc.format, tc.pin, err)
		}
		got, err := DecodePINBlock(tc.format, block, pan)
		if err != nil {
			t.Fatalf("Decode(%d): %v", tc.format, err)
		}
		if got != tc.pin {
			t.Errorf("format %d round-trip = %q want %q", tc.format, got, tc.pin)
		}
	}
}

func TestPINBlockErrors(t *testing.T) {
	if _, err := EncodePINBlock(ISO0, "12", "4012345678909"); err == nil {
		t.Error("short PIN accepted")
	}
	if _, err := EncodePINBlock(ISO0, "12A4", "4012345678909"); err == nil {
		t.Error("non-decimal PIN accepted")
	}
	if _, err := EncodePINBlock(ISO0, "1234", "123"); err == nil {
		t.Error("short PAN accepted")
	}
	// Format mismatch on decode.
	block, _ := EncodePINBlock(ISO0, "1234", "4012345678909")
	if _, err := DecodePINBlock(ISO1, block, "4012345678909"); err == nil {
		t.Error("format-mismatched decode accepted")
	}
}

func TestPINBlockPaddingValidation(t *testing.T) {
	pan := "4012345678909"
	// A clear ISO-0 block with valid control/length/digits but a non-F pad
	// nibble must be rejected (decode XORs the PAN field back in, so craft the
	// clear block then XOR the PAN field to get the input).
	clear := unpackNibbles(make([]byte, 8))
	clear[0] = 0x0 // ISO-0 control
	clear[1] = 0x4 // length 4
	clear[2], clear[3], clear[4], clear[5] = 1, 2, 3, 4
	for i := 6; i < 16; i++ {
		clear[i] = 0xF
	}
	clear[15] = 0x3 // corrupt one pad nibble (not F)
	af, _ := panField(pan)
	bad := xored(packNibbles(clear), af)
	if _, err := DecodePINBlock(ISO0, bad, pan); err == nil {
		t.Error("ISO-0 block with non-F padding was accepted")
	}

	// ISO-3 must reject a decimal (0-9) pad nibble.
	clear3 := unpackNibbles(make([]byte, 8))
	clear3[0] = 0x3
	clear3[1] = 0x4
	clear3[2], clear3[3], clear3[4], clear3[5] = 5, 6, 7, 8
	for i := 6; i < 16; i++ {
		clear3[i] = 0xC // valid A-F
	}
	clear3[10] = 0x2 // decimal pad → invalid for ISO-3
	bad3 := xored(packNibbles(clear3), af)
	if _, err := DecodePINBlock(ISO3, bad3, pan); err == nil {
		t.Error("ISO-3 block with decimal padding was accepted")
	}
}

func TestMACRoundTripAndVerify(t *testing.T) {
	data := []byte("0200ISO8583 MAC TEST DATA BLOCK")
	for _, alg := range []struct {
		a   MACAlgorithm
		key []byte
	}{
		{MACAlg1, mustHex(t, "0123456789ABCDEF")},
		{MACAlg3, mustHex(t, "0123456789ABCDEFFEDCBA9876543210")},
	} {
		for _, pad := range []Padding{Pad1, Pad2} {
			mac, err := GenerateMAC(alg.a, pad, alg.key, data)
			if err != nil {
				t.Fatalf("GenerateMAC(alg=%d,pad=%d): %v", alg.a, pad, err)
			}
			if len(mac) != 8 {
				t.Errorf("MAC length = %d want 8", len(mac))
			}
			ok, err := VerifyMAC(alg.a, pad, alg.key, data, mac)
			if err != nil || !ok {
				t.Errorf("VerifyMAC = %v,%v want true", ok, err)
			}
			// Truncated MAC (4 bytes) must verify too.
			ok, err = VerifyMAC(alg.a, pad, alg.key, data, mac[:4])
			if err != nil || !ok {
				t.Errorf("VerifyMAC trunc = %v,%v want true", ok, err)
			}
			// Tampered data must fail.
			ok, _ = VerifyMAC(alg.a, pad, alg.key, append(data, 'x'), mac)
			if ok {
				t.Errorf("VerifyMAC accepted tampered data")
			}
		}
	}
}

func TestMACPaddingDistinct(t *testing.T) {
	// Pad1 and Pad2 differ for an already block-aligned message.
	key := mustHex(t, "0123456789ABCDEF")
	data := make([]byte, 16) // 2 full blocks
	m1, _ := GenerateMAC(MACAlg1, Pad1, key, data)
	m2, _ := GenerateMAC(MACAlg1, Pad2, key, data)
	if bytes.Equal(m1, m2) {
		t.Error("Pad1 and Pad2 produced the same MAC on aligned input")
	}
}

func TestTR31RoundTrip(t *testing.T) {
	kbpk := mustHex(t, "89E88CF7931444F334BD7547FC3F380C") // 16-byte KBPK
	key := mustHex(t, "F039121BEC83D26B169BDCD5B22AAF8F")  // a 3DES working key
	hdr := KeyBlockHeader{KeyUsage: "P0", Algorithm: 'T', ModeOfUse: 'E', KeyVersion: "00", Exportability: 'N'}

	block, err := WrapKeyBlockRand(kbpk, hdr, key, constReader(0))
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	if block[0] != 'B' {
		t.Errorf("version = %q want B", block[0])
	}
	if l, _ := atoiFixed(block[1:5]); l != len(block) {
		t.Errorf("header length %d != block length %d", l, len(block))
	}

	gotHdr, gotKey, err := UnwrapKeyBlock(kbpk, block)
	if err != nil {
		t.Fatalf("Unwrap: %v", err)
	}
	if !bytes.Equal(gotKey, key) {
		t.Errorf("recovered key = %X want %X", gotKey, key)
	}
	if gotHdr.KeyUsage != "P0" || gotHdr.Algorithm != 'T' || gotHdr.ModeOfUse != 'E' || gotHdr.Exportability != 'N' {
		t.Errorf("recovered header = %+v", gotHdr)
	}
}

func TestTR31TamperAndWrongKey(t *testing.T) {
	kbpk := mustHex(t, "89E88CF7931444F334BD7547FC3F380C")
	key := mustHex(t, "F039121BEC83D26B169BDCD5B22AAF8F")
	hdr := KeyBlockHeader{KeyUsage: "B0", Algorithm: 'T', ModeOfUse: 'N', Exportability: 'N'}
	block, err := WrapKeyBlockRand(kbpk, hdr, key, constReader(0x5A))
	if err != nil {
		t.Fatal(err)
	}

	// Flip a byte in the encrypted body: authentication must fail.
	bad := []byte(block)
	bad[20] ^= 0x01
	if _, _, err := UnwrapKeyBlock(kbpk, string(bad)); err == nil {
		t.Error("tampered block authenticated")
	}

	// Wrong KBPK must fail authentication.
	wrong := mustHex(t, "00000000000000000000000000000000")
	if _, _, err := UnwrapKeyBlock(wrong, block); err == nil {
		t.Error("wrong KBPK authenticated")
	}
}

func TestEMVARQCARPC(t *testing.T) {
	mk := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	atc := mustHex(t, "0001")
	sk, err := DeriveEMVSessionKey(mk, atc)
	if err != nil {
		t.Fatalf("DeriveEMVSessionKey: %v", err)
	}
	if len(sk) != 16 {
		t.Fatalf("session key length = %d want 16", len(sk))
	}
	// Determinism.
	sk2, _ := DeriveEMVSessionKey(mk, atc)
	if !bytes.Equal(sk, sk2) {
		t.Error("session key derivation not deterministic")
	}
	// A different ATC yields a different key.
	skOther, _ := DeriveEMVSessionKey(mk, mustHex(t, "0002"))
	if bytes.Equal(sk, skOther) {
		t.Error("different ATC gave same session key")
	}

	data := mustHex(t, "000000001000000000000000084000000000000840000000000000000000000000")
	arqc, err := GenerateARQC(sk, data)
	if err != nil {
		t.Fatalf("GenerateARQC: %v", err)
	}
	if ok, _ := VerifyARQC(sk, data, arqc); !ok {
		t.Error("VerifyARQC rejected a valid ARQC")
	}
	if ok, _ := VerifyARQC(sk, append(data, 0x00), arqc); ok {
		t.Error("VerifyARQC accepted tampered data")
	}

	arpc, err := GenerateARPC(sk, arqc, mustHex(t, "3030"))
	if err != nil {
		t.Fatalf("GenerateARPC: %v", err)
	}
	if len(arpc) != 8 {
		t.Errorf("ARPC length = %d want 8", len(arpc))
	}
	// ARPC method 1 relationship: ARPC = 3DES(SK, ARQC ⊕ (ARC‖0)).
	b, _ := tripleDESBlock(sk)
	padded := make([]byte, 8)
	copy(padded, mustHex(t, "3030"))
	if want := blockEnc(b, xored(arqc, padded)); !bytes.Equal(arpc, want) {
		t.Errorf("ARPC = %X want %X", arpc, want)
	}
}

func TestSoftVaultPINTranslateAndMAC(t *testing.T) {
	v := NewSoftVault()
	pan := "4012345678909"
	zpk1 := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	zpk2 := mustHex(t, "FEDCBA98765432100123456789ABCDEF")
	v.SetKey("ZPK1", zpk1)
	v.SetKey("ZPK2", zpk2)

	enc, err := v.EncryptPINBlock("ZPK1", ISO0, "1234", pan)
	if err != nil {
		t.Fatalf("EncryptPINBlock: %v", err)
	}
	// Translate ZPK1 -> ZPK2 (same format), then decrypt under ZPK2 and check PIN.
	trans, err := v.TranslatePIN("ZPK1", "ZPK2", enc, pan, ISO0, ISO0)
	if err != nil {
		t.Fatalf("TranslatePIN: %v", err)
	}
	if bytes.Equal(enc, trans) {
		t.Error("translation under a different key produced the same ciphertext")
	}
	b, _ := tripleDESBlock(zpk2)
	clear, _ := ecbDecrypt(b, trans)
	if pin, _ := DecodePINBlock(ISO0, clear, pan); pin != "1234" {
		t.Errorf("translated PIN = %q want 1234", pin)
	}

	// MAC via the vault.
	v.SetKey("MACKEY", mustHex(t, "0123456789ABCDEFFEDCBA9876543210"))
	data := []byte("mac me")
	mac, err := v.GenerateMAC("MACKEY", MACAlg3, Pad2, data)
	if err != nil {
		t.Fatalf("GenerateMAC: %v", err)
	}
	if ok, _ := v.VerifyMAC("MACKEY", MACAlg3, Pad2, data, mac); !ok {
		t.Error("vault VerifyMAC rejected its own MAC")
	}

	if _, err := v.EncryptPINBlock("NOPE", ISO0, "1234", pan); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("unknown key err = %v want ErrUnknownKey", err)
	}
}

func TestSoftVaultImportKeyBlock(t *testing.T) {
	kbpk := mustHex(t, "89E88CF7931444F334BD7547FC3F380C")
	key := mustHex(t, "0123456789ABCDEFFEDCBA9876543210")
	block, err := WrapKeyBlock(kbpk, KeyBlockHeader{KeyUsage: "M0", Algorithm: 'T', ModeOfUse: 'B', Exportability: 'N'}, key)
	if err != nil {
		t.Fatal(err)
	}
	v := NewSoftVault()
	hdr, err := v.ImportKeyBlock("M0", kbpk, block)
	if err != nil {
		t.Fatalf("ImportKeyBlock: %v", err)
	}
	if hdr.KeyUsage != "M0" {
		t.Errorf("imported usage = %q want M0", hdr.KeyUsage)
	}
	// The imported key must work for MAC, matching a direct computation.
	data := []byte("hello")
	viaVault, err := v.GenerateMAC("M0", MACAlg3, Pad2, data)
	if err != nil {
		t.Fatal(err)
	}
	direct, _ := GenerateMAC(MACAlg3, Pad2, key, data)
	if !bytes.Equal(viaVault, direct) {
		t.Error("imported key MAC differs from direct computation")
	}
}
