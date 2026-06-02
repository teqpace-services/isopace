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

package packager_test

import (
	"bytes"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
)

// TestCoralPayZoneRoundTrip builds a representative 0200 with each generated
// packager and round-trips it through Marshal/Unmarshal, asserting the MTI, a few
// ASCII fields, and the 8-byte binary PIN block (F52) survive — CoralPay carries
// F52 as ASCII-hex (IFA_BINARY) and Zone as raw octets (IFB_BINARY), so this
// exercises both binary disciplines.
func TestCoralPayZoneRoundTrip(t *testing.T) {
	pin := []byte{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0}

	for _, tc := range []struct {
		name   string
		schema *iso8583.Schema
	}{
		{"CoralPay", packager.CoralPay()},
		{"Zone", packager.Zone()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			codec := iso8583.NewCodec(tc.schema)
			m := iso8583.New(tc.schema)
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("set: %v", err)
				}
			}
			must(m.Set(0, "0200"))
			must(m.Set(2, "5060990580000217499"))
			must(m.Set(3, "010000"))
			must(m.Set(4, "000000150000"))
			must(m.Set(11, "000123"))
			must(m.Set(41, "20405060"))
			must(m.Set(49, "566"))
			must(m.Set(52, pin))

			wire, err := codec.Marshal(m, nil)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			got, err := codec.Unmarshal(wire)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if mti, _ := got.MTI(); mustStr(t, mti) != "0200" {
				t.Errorf("MTI = %q, want 0200", mustStr(t, mti))
			}
			if v, ok := got.Get(2); !ok || mustStr(t, v) != "5060990580000217499" {
				t.Errorf("F2 = %q", mustStr(t, v))
			}
			if v, ok := got.Get(11); !ok || mustStr(t, v) != "000123" {
				t.Errorf("F11 = %q", mustStr(t, v))
			}
			if v, ok := got.Get(52); !ok || !bytes.Equal(v.Bytes(), pin) {
				t.Errorf("F52 = % X, want % X", v.Bytes(), pin)
			}
		})
	}
}

func mustStr(t *testing.T, v iso8583.Value) string {
	t.Helper()
	s, err := v.String()
	if err != nil {
		t.Fatalf("value.String: %v", err)
	}
	return s
}
