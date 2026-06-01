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

	"github.com/teqpace-services/isopace/fieldcodec"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
)

func TestGenericLoaderRoundTrip(t *testing.T) {
	s, err := packager.LoadEmbedded("iso87b.json")
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	if s.ID() != "iso87-b-generic" {
		t.Errorf("schema id = %q", s.ID())
	}

	c := iso8583.NewCodec(s)
	m := iso8583.New(s)
	set(t, m, 0, "0200")
	set(t, m, 2, pan)
	set(t, m, 3, "000000")
	set(t, m, 4, iso8583.NewDecimal(2599, 0))
	set(t, m, 11, int64(456))
	setS(t, m, "55.9F26", []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88})
	setS(t, m, "55.9F36", int64(3))

	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got, _ := iso8583.Get[string](m2, 2); got != pan {
		t.Errorf("PAN = %q", got)
	}
	if got, _ := iso8583.Get[int64](m2, 11); got != 456 {
		t.Errorf("STAN = %d want 456", got)
	}
	if amt, _ := iso8583.Get[iso8583.Decimal](m2, 4); amt.Unscaled != 2599 {
		t.Errorf("amount = %d want 2599", amt.Unscaled)
	}
	if cg, _ := iso8583.GetS[[]byte](m2, "55.9F26"); !bytes.Equal(cg, []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}) {
		t.Errorf("55.9F26 = % X", cg)
	}

	wire2, _ := c.Marshal(m2, nil)
	if !bytes.Equal(wire, wire2) {
		t.Errorf("generic schema round-trip not stable")
	}
}

func TestGenericLoaderErrors(t *testing.T) {
	reg := fieldcodec.DefaultRegistry()
	bad := []byte(`{"id":"x","mti":"no.such.codec","bitmap":{"codec":"bitmap.bin","levels":2},"fields":{}}`)
	if _, err := packager.Load(bad, reg); err == nil {
		t.Errorf("expected error for unknown MTI codec")
	}
	badField := []byte(`{"id":"x","mti":"num.ascii","bitmap":{"codec":"bitmap.bin","levels":2},"fields":{"2":{"codec":"nope","max":5}}}`)
	if _, err := packager.Load(badField, reg); err == nil {
		t.Errorf("expected error for unknown field codec")
	}
}
