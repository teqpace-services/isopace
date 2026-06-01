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

package tlv_test

import (
	"testing"

	"github.com/teqpace-services/isopace/fieldcodec/tlv"
	"github.com/teqpace-services/isopace/iso8583"
)

// FuzzBERTLV drives arbitrary bytes through the BER-TLV parser (DE 55 is
// attacker-influenced wire). It must never panic, and any successful decode must
// re-encode and re-decode stably.
func FuzzBERTLV(f *testing.F) {
	def := &iso8583.FieldDef{Sub: emvSchema(), Kind: iso8583.KindComposite}
	f.Add([]byte{0x9F, 0x26, 0x08, 0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18})
	f.Add([]byte{0x95, 0x05, 0x80, 0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := tlv.BERTLV.DecodeBody(data, len(data), def)
		if err != nil {
			return // malformed TLV is expected; must not panic
		}
		out, err := tlv.BERTLV.EncodeBody(nil, v, def)
		if err != nil {
			t.Fatalf("EncodeBody after successful decode: %v", err)
		}
		if _, err := tlv.BERTLV.DecodeBody(out, len(out), def); err != nil {
			t.Fatalf("re-decode of encoded TLV failed: %v", err)
		}
	})
}
