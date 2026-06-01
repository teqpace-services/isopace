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

package protobuf_test

import (
	"bytes"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/render/protobuf"
)

const pan = "4111111111111111"

func TestProtobufRoundTrip(t *testing.T) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)

	m := iso8583.New(s)
	for _, kv := range []struct {
		de int
		v  any
	}{
		{0, "0200"}, {2, pan}, {3, "000000"}, {4, iso8583.NewDecimal(2599, 0)},
		{11, int64(456)}, {41, "TERM0001"},
	} {
		if err := m.Set(kv.de, kv.v); err != nil {
			t.Fatalf("Set(%d): %v", kv.de, err)
		}
	}
	if err := m.SetS("55.9F26", []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33}); err != nil {
		t.Fatalf("SetS: %v", err)
	}

	pb, err := protobuf.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := protobuf.Unmarshal(pb, s)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	wireA, _ := c.Marshal(m, nil)
	wireB, _ := c.Marshal(m2, nil)
	if !bytes.Equal(wireA, wireB) {
		t.Errorf("protobuf round-trip not wire-identical:\n A=%x\n B=%x", wireA, wireB)
	}

	// EMV composite survives the protobuf hop.
	if cg, _ := iso8583.GetS[[]byte](m2, "55.9F26"); !bytes.Equal(cg, []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33}) {
		t.Errorf("55.9F26 lost across protobuf: % X", cg)
	}
}

func TestProtobufRejectsGarbage(t *testing.T) {
	s := packager.ISO87A()
	// Truncated length-delimited field must error, not panic.
	if _, err := protobuf.Unmarshal([]byte{0x12, 0xFF}, s); err == nil {
		t.Errorf("expected error on truncated protobuf")
	}
}
