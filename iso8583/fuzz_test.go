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

package iso8583

import (
	"bytes"
	"testing"
)

// FuzzParsePath asserts the path parser never panics and that the canonical
// String form re-parses to an equal path.
func FuzzParsePath(f *testing.F) {
	for _, s := range []string{"2", "48.2.1", "55.9F26", "0", "1.2.3.4.5.6"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, err := ParsePath(s)
		if err != nil {
			return
		}
		p2, err := ParsePath(p.String())
		if err != nil {
			t.Fatalf("canonical %q failed to re-parse: %v", p.String(), err)
		}
		if p2 != p {
			t.Errorf("round-trip mismatch: %q -> %v -> %q -> %v", s, p, p.String(), p2)
		}
	})
}

// FuzzUnmarshalMarshal asserts the engine never panics on arbitrary input and
// that marshal-after-unmarshal is stable (idempotent re-encode).
func FuzzUnmarshalMarshal(f *testing.F) {
	s := testSchema()
	c := NewCodec(s)

	seed := New(s)
	_ = seed.Set(0, "0200")
	_ = seed.Set(2, validPAN)
	_ = seed.Set(3, "0")
	_ = seed.Set(4, NewDecimal(1099, 2))
	_ = seed.Set(11, int64(123))
	if wire, err := c.Marshal(seed, nil); err == nil {
		f.Add(wire)
	}
	f.Add([]byte("0200"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := c.Unmarshal(data)
		if err != nil {
			return // malformed input is expected; just must not panic
		}
		out, err := c.Marshal(m, nil)
		if err != nil {
			t.Fatalf("Marshal after successful Unmarshal failed: %v", err)
		}
		m2, err := c.Unmarshal(out)
		if err != nil {
			t.Fatalf("re-Unmarshal of marshalled output failed: %v", err)
		}
		out2, err := c.Marshal(m2, nil)
		if err != nil {
			t.Fatalf("re-Marshal failed: %v", err)
		}
		if !bytes.Equal(out, out2) {
			t.Errorf("marshal not idempotent:\n%x\n%x", out, out2)
		}
	})
}
