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

const pan = "4111111111111111"

// profiles under test (excluding EMV/overlay specifics, exercised separately).
func allProfiles() map[string]*iso8583.Schema {
	return map[string]*iso8583.Schema{
		"iso87-a":    packager.ISO87A(),
		"iso87-b":    packager.ISO87B(),
		"iso87-c":    packager.ISO87C(),
		"iso93-a":    packager.ISO93A(),
		"iso93-b":    packager.ISO93B(),
		"visa-base1": packager.VisaBaseI(),
		"mastercard": packager.Mastercard(),
	}
}

func TestProfileRoundTrip(t *testing.T) {
	for id, s := range allProfiles() {
		t.Run(id, func(t *testing.T) {
			c := iso8583.NewCodec(s)
			m := iso8583.New(s)
			set(t, m, 0, "0200")
			set(t, m, 2, pan)
			set(t, m, 3, "000000")
			set(t, m, 4, iso8583.NewDecimal(1099, 0))
			set(t, m, 11, int64(123))
			set(t, m, 41, "TERM0001")

			wire, err := c.Marshal(m, nil)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			m2, err := c.Unmarshal(wire)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if got, _ := iso8583.Get[string](m2, 0); got != "0200" {
				t.Errorf("MTI = %q", got)
			}
			if got, _ := iso8583.Get[string](m2, 2); got != pan {
				t.Errorf("PAN = %q want %q", got, pan)
			}
			if got, _ := iso8583.Get[int64](m2, 11); got != 123 {
				t.Errorf("STAN = %d want 123", got)
			}
			amt, _ := iso8583.Get[iso8583.Decimal](m2, 4)
			if amt.Unscaled != 1099 {
				t.Errorf("amount unscaled = %d want 1099", amt.Unscaled)
			}

			// Re-marshalling the decoded message reproduces the wire.
			wire2, err := c.Marshal(m2, nil)
			if err != nil {
				t.Fatalf("re-Marshal: %v", err)
			}
			if !bytes.Equal(wire, wire2) {
				t.Errorf("%s not stable across round-trip", id)
			}
		})
	}
}

func TestISO87EMV(t *testing.T) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	m := iso8583.New(s)
	set(t, m, 0, "0200")
	set(t, m, 2, pan)
	cryptogram := []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18}
	setS(t, m, "55.9F26", cryptogram)
	setS(t, m, "55.9F36", int64(7))
	setS(t, m, "55.95", []byte{0x80, 0x00, 0x00, 0x00, 0x00})

	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	got, err := iso8583.GetS[[]byte](m2, "55.9F26")
	if err != nil || !bytes.Equal(got, cryptogram) {
		t.Errorf("55.9F26 = % X, %v want % X", got, err, cryptogram)
	}
	if atc, _ := iso8583.GetS[int64](m2, "55.9F36"); atc != 7 {
		t.Errorf("55.9F36 ATC = %d want 7", atc)
	}
}

func TestDeriveOverlayDistinct(t *testing.T) {
	// Visa Base I redefines DE 63 as binary; the base ISO93A leaves it textual.
	base := packager.ISO93A()
	visa := packager.VisaBaseI()
	if bd, _ := base.Field(63); bd.Kind != iso8583.KindString {
		t.Errorf("base DE63 kind = %v want string", bd.Kind)
	}
	if vd, _ := visa.Field(63); vd.Kind != iso8583.KindBytes {
		t.Errorf("visa DE63 kind = %v want bytes (overlay)", vd.Kind)
	}

	// And the binary DE63 round-trips on the Visa profile.
	c := iso8583.NewCodec(visa)
	m := iso8583.New(visa)
	set(t, m, 0, "0210")
	set(t, m, 63, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, _ := c.Unmarshal(wire)
	if got, _ := iso8583.Get[[]byte](m2, 63); !bytes.Equal(got, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("visa DE63 = % X", got)
	}
}

func TestISO93ResponseCodeWidth(t *testing.T) {
	// 1993 widened the response code (DE 39) to 3 characters.
	d87, _ := packager.ISO87A().Field(39)
	d93, _ := packager.ISO93A().Field(39)
	if d87.MaxLen != 2 || d93.MaxLen != 3 {
		t.Errorf("DE39 widths: 87=%d (want 2) 93=%d (want 3)", d87.MaxLen, d93.MaxLen)
	}
}

func set(t *testing.T, m *iso8583.Message, de int, v any) {
	t.Helper()
	if err := m.Set(de, v); err != nil {
		t.Fatalf("Set(%d): %v", de, err)
	}
}

func setS(t *testing.T, m *iso8583.Message, path string, v any) {
	t.Helper()
	if err := m.SetS(path, v); err != nil {
		t.Fatalf("SetS(%q): %v", path, err)
	}
}
