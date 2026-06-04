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

package pkcs11

import (
	"bytes"
	"testing"

	"github.com/teqpace-services/isopace/vault"
)

// iso9797Pad must mirror vault's ISO 9797-1 padding exactly — the SoftHSM
// cross-check relies on the HSM seeing the same padded bytes the software
// reference MACs. These vectors lock the behaviour even without a token.
func TestISO9797Pad(t *testing.T) {
	cases := []struct {
		name string
		pad  vault.Padding
		in   []byte
		want []byte
	}{
		{"pad1 empty", vault.Pad1, nil, []byte{0, 0, 0, 0, 0, 0, 0, 0}},
		{"pad1 aligned", vault.Pad1, []byte{1, 2, 3, 4, 5, 6, 7, 8}, []byte{1, 2, 3, 4, 5, 6, 7, 8}},
		{"pad1 partial", vault.Pad1, []byte{1, 2, 3}, []byte{1, 2, 3, 0, 0, 0, 0, 0}},
		{"pad2 empty", vault.Pad2, nil, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}},
		{"pad2 aligned", vault.Pad2, []byte{1, 2, 3, 4, 5, 6, 7, 8},
			[]byte{1, 2, 3, 4, 5, 6, 7, 8, 0x80, 0, 0, 0, 0, 0, 0, 0}},
		{"pad2 partial", vault.Pad2, []byte{1, 2, 3}, []byte{1, 2, 3, 0x80, 0, 0, 0, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := iso9797Pad(c.pad, c.in)
			if len(got)%desBlockSize != 0 {
				t.Fatalf("not block-aligned: %d bytes", len(got))
			}
			if !bytes.Equal(got, c.want) {
				t.Fatalf("iso9797Pad = % x, want % x", got, c.want)
			}
		})
	}
}
