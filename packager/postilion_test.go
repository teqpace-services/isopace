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
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
)

// TestPostilionRoundTrip builds a representative 0200 (including the LLLCHAR DE
// 56/123, a track-2 with a 'D' separator, and the DE 127 subfield group) and
// asserts it marshals to a BINARY-bitmap wire and unmarshals back losslessly.
func TestPostilionRoundTrip(t *testing.T) {
	s := packager.Postilion()
	c := iso8583.NewCodec(s)
	m := iso8583.New(s)

	fields := []struct {
		de  int
		val string
	}{
		{0, "0200"},
		{2, "5399410000000000"},
		{3, "500000"},
		{4, "000000005000"},
		{7, "0601142407"},
		{11, "000029"},
		{28, "D00000000"}, // IFA_AMOUNT-style, carried as fixed-9 text
		{35, "5399410000000000D2901999999999999"}, // track 2 (has 'D')
		{49, "566"},
		{56, "1510"},             // LLLCHAR (not fixed)
		{123, "510111511344101"}, // LLLCHAR (not fixed)
	}
	for _, f := range fields {
		if err := m.Set(f.de, f.val); err != nil {
			t.Fatalf("set DE %d: %v", f.de, err)
		}
	}
	sub := []struct{ path, val string }{
		{"127.2", "0200:415495:1207193655:787755594"}, // LLCHAR (len 32)
		{"127.20", "20260601"},
		{"127.25", "<IccData>test</IccData>"}, // LLLLCHAR
		{"127.41", "10.2.103.19,36065"},       // LLCHAR
	}
	for _, f := range sub {
		if err := m.SetS(f.path, f.val); err != nil {
			t.Fatalf("set %s: %v", f.path, err)
		}
	}

	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// MTI is 4 ASCII bytes; the next byte is the first bitmap byte. With DE 127
	// present, the continuation bit (0x80) must be set — proving a BINARY bitmap
	// (an ASCII-hex bitmap byte would be < 0x80).
	if len(wire) < 5 || wire[4] < 0x80 {
		t.Fatalf("expected binary bitmap with continuation bit set; wire[4]=%#x", wire[4])
	}

	got, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, f := range fields {
		if f.de == 0 {
			continue
		}
		v, gerr := iso8583.Get[string](got, f.de)
		if gerr != nil {
			t.Fatalf("get DE %d: %v", f.de, gerr)
		}
		if v != f.val {
			t.Errorf("DE %d round-trip: got %q want %q", f.de, v, f.val)
		}
	}
	for _, f := range sub {
		v, gerr := iso8583.GetS[string](got, f.path)
		if gerr != nil {
			t.Fatalf("get %s: %v", f.path, gerr)
		}
		if v != f.val {
			t.Errorf("%s round-trip: got %q want %q", f.path, v, f.val)
		}
	}
}

// TestPostilionRegistered checks the profile is discoverable by id.
func TestPostilionRegistered(t *testing.T) {
	if _, ok := packager.Profile("postilion"); !ok {
		t.Fatal(`Profile("postilion") not registered`)
	}
}
