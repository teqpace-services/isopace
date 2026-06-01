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

package conformance

import (
	"bytes"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
)

// FuzzProfileRoundTrip drives arbitrary input through the full ISO-8583
// pipeline (structural unmarshal, lazy per-field decode incl. BER-TLV, and
// marshal). It asserts the engine never panics and that marshal-after-unmarshal
// is stable. Seeded with the golden vectors.
func FuzzProfileRoundTrip(f *testing.F) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	for _, v := range vectors() {
		f.Add(v.wire)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := c.Unmarshal(data)
		if err != nil {
			return // malformed input is expected; must not panic
		}
		// Force lazy decode of every present field (exercises codecs + TLV).
		for range m.Fields() {
		}
		out, err := c.Marshal(m, nil)
		if err != nil {
			t.Fatalf("Marshal after successful Unmarshal: %v", err)
		}
		m2, err := c.Unmarshal(out)
		if err != nil {
			t.Fatalf("re-Unmarshal failed: %v", err)
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
