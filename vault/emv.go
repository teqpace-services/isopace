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
	"crypto/subtle"
	"fmt"
)

// DeriveEMVSessionKey derives an EMV application cryptogram session key from a
// 16-byte Application Cryptogram master key and a 2-byte Application Transaction
// Counter, using the EMV Common Session Key method (EMV Book 2, Annex A1.3):
// the two halves are MK-encryptions of the ATC tagged with 0xF0 and 0x0F.
func DeriveEMVSessionKey(mk, atc []byte) ([]byte, error) {
	if len(mk) != 16 {
		return nil, fmt.Errorf("vault: EMV master key must be 16 bytes, got %d", len(mk))
	}
	if len(atc) != 2 {
		return nil, fmt.Errorf("vault: ATC must be 2 bytes, got %d", len(atc))
	}
	b, err := tripleDESBlock(mk)
	if err != nil {
		return nil, err
	}
	f1 := []byte{atc[0], atc[1], 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00}
	f2 := []byte{atc[0], atc[1], 0x0F, 0x00, 0x00, 0x00, 0x00, 0x00}
	return append(blockEnc(b, f1), blockEnc(b, f2)...), nil
}

// GenerateARQC computes the Application Request Cryptogram over the supplied
// transaction data with the session key: ISO 9797-1 MAC algorithm 3 (retail MAC)
// with padding method 2, as EMV specifies. The caller assembles data from the
// agreed CDOL data elements.
func GenerateARQC(sessionKey, data []byte) ([]byte, error) {
	return GenerateMAC(MACAlg3, Pad2, sessionKey, data)
}

// VerifyARQC recomputes the ARQC over data and constant-time compares it.
func VerifyARQC(sessionKey, data, arqc []byte) (bool, error) {
	want, err := GenerateARQC(sessionKey, data)
	if err != nil {
		return false, err
	}
	if len(arqc) != len(want) {
		return false, nil
	}
	return subtle.ConstantTimeCompare(want, arqc) == 1, nil
}

// GenerateARPC computes the Authorisation Response Cryptogram by EMV method 1:
// ARPC = 3DES(sessionKey, ARQC ⊕ (ARC ‖ zeros)), where ARC is the 2-byte
// Authorisation Response Code placed in the leftmost bytes.
func GenerateARPC(sessionKey, arqc, arc []byte) ([]byte, error) {
	if len(arqc) != 8 {
		return nil, fmt.Errorf("vault: ARQC must be 8 bytes, got %d", len(arqc))
	}
	if len(arc) != 2 {
		return nil, fmt.Errorf("vault: ARC must be 2 bytes, got %d", len(arc))
	}
	b, err := tripleDESBlock(sessionKey)
	if err != nil {
		return nil, err
	}
	padded := make([]byte, 8)
	copy(padded, arc)
	return blockEnc(b, xored(arqc, padded)), nil
}
