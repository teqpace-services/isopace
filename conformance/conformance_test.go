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
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
)

// updateGolden regenerates the testdata/*.hex golden files from the in-code
// vectors: `go test ./conformance -update`.
var updateGolden = flag.Bool("update", false, "regenerate golden vector files")

// vector is one hand-derived ISO-8583 golden message. The wire bytes are
// constructed from the ISO 8583:1987 structure (MTI + bitmap + ordered fields),
// independent of the library's encoder, so decode/encode conformance is a real
// check rather than a tautology.
type vector struct {
	name   string
	file   string
	schema *iso8583.Schema
	wire   []byte
	// checks asserts the decoded logical content.
	checks func(t *testing.T, m *iso8583.Message)
}

func vectors() []vector {
	iso87a := packager.ISO87A()
	return []vector{
		{
			name:   "iso87a-0200-auth",
			file:   "iso87a_0200_auth.hex",
			schema: iso87a,
			// MTI 0200 | primary bitmap DE{2,3,4,11,41} | DE2 LLVAR PAN |
			// DE3 proc | DE4 amount(12) | DE11 STAN(6) | DE41 terminal(8).
			wire: []byte("0200" +
				"7020000000800000" + // DE 2,3,4 (0x70) · 11 (byte1 0x20) · 41 (byte5 0x80)
				"16" + "4111111111111111" +
				"000000" +
				"000000001099" +
				"000123" +
				"TERM0001"),
			checks: func(t *testing.T, m *iso8583.Message) {
				assertStr(t, m, 0, "0200")
				assertStr(t, m, 2, "4111111111111111")
				assertStr(t, m, 41, "TERM0001")
				if n, _ := iso8583.Get[int64](m, 11); n != 123 {
					t.Errorf("DE11 = %d want 123", n)
				}
				if amt, _ := iso8583.Get[iso8583.Decimal](m, 4); amt.Unscaled != 1099 {
					t.Errorf("DE4 = %d want 1099", amt.Unscaled)
				}
			},
		},
		{
			name:   "iso87a-0210-response",
			file:   "iso87a_0210_response.hex",
			schema: iso87a,
			// As above plus DE39 response code; bitmap adds bit 39 (byte4 0x02).
			wire: []byte("0210" +
				"7020000002800000" +
				"16" + "4111111111111111" +
				"000000" +
				"000000001099" +
				"000123" +
				"00" + // DE39 response code
				"TERM0001"),
			checks: func(t *testing.T, m *iso8583.Message) {
				assertStr(t, m, 0, "0210")
				assertStr(t, m, 39, "00")
				assertStr(t, m, 2, "4111111111111111")
			},
		},
	}
}

func TestGoldenVectors(t *testing.T) {
	for _, v := range vectors() {
		t.Run(v.name, func(t *testing.T) {
			golden := readOrUpdateGolden(t, v.file, v.wire)
			if !bytes.Equal(golden, v.wire) {
				t.Fatalf("in-code vector diverged from golden file %s (run -update if intentional)", v.file)
			}

			c := iso8583.NewCodec(v.schema)
			m, err := c.Unmarshal(v.wire)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			v.checks(t, m)

			// Re-encode must reproduce the golden wire byte-for-byte.
			out, err := c.Marshal(m, nil)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if !bytes.Equal(out, v.wire) {
				t.Errorf("re-encode mismatch:\n got %s\nwant %s", hex.EncodeToString(out), hex.EncodeToString(v.wire))
			}
		})
	}
}

// readOrUpdateGolden writes the golden file when -update is set, else reads it.
func readOrUpdateGolden(t *testing.T, name string, wire []byte) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(hex.EncodeToString(wire)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return wire
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run `go test ./conformance -update` to create): %v", name, err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("decode golden %s: %v", name, err)
	}
	return b
}

func assertStr(t *testing.T, m *iso8583.Message, de int, want string) {
	t.Helper()
	got, err := iso8583.Get[string](m, de)
	if err != nil {
		t.Errorf("DE %d: %v", de, err)
		return
	}
	if got != want {
		t.Errorf("DE %d = %q want %q", de, got, want)
	}
}
