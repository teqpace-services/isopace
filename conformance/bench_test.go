// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction package.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.

package conformance

import (
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
)

func benchWire() (*iso8583.Codec, []byte) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	for _, v := range vectors() {
		if v.name == "iso87a-0200-auth" {
			return c, v.wire
		}
	}
	return c, nil
}

// BenchmarkUnmarshal measures the zero-copy structural parse (no field bodies
// decoded).
func BenchmarkUnmarshal(b *testing.B) {
	c, wire := benchWire()
	b.ReportAllocs()
	b.SetBytes(int64(len(wire)))
	for i := 0; i < b.N; i++ {
		if _, err := c.Unmarshal(wire); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRouteHotPath models a switch routing on a few fields without
// decoding the rest.
func BenchmarkRouteHotPath(b *testing.B) {
	c, wire := benchWire()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m, err := c.Unmarshal(wire)
		if err != nil {
			b.Fatal(err)
		}
		if v, ok := m.Get(3); ok {
			_ = v.Bytes() // proc code, zero-copy
		}
		_ = m.Has(41)
	}
}

// BenchmarkMarshalCleanFastPath measures store-and-forward: an unmodified
// message re-marshals by returning src verbatim — expected zero allocations.
func BenchmarkMarshalCleanFastPath(b *testing.B) {
	c, wire := benchWire()
	m, err := c.Unmarshal(wire)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(wire)))
	for i := 0; i < b.N; i++ {
		if _, err := c.Marshal(m, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMarshalRebuild measures a full re-encode of a freshly built message.
func BenchmarkMarshalRebuild(b *testing.B) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	m := iso8583.New(s)
	_ = m.Set(0, "0200")
	_ = m.Set(2, "4111111111111111")
	_ = m.Set(3, "000000")
	_ = m.Set(4, iso8583.NewDecimal(1099, 0))
	_ = m.Set(11, int64(123))
	_ = m.Set(41, "TERM0001")

	buf := make([]byte, 0, 128)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := c.Marshal(m, buf[:0]); err != nil {
			b.Fatal(err)
		}
	}
}
