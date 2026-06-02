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

package iso8583

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// authMsg builds a representative authorisation request over testSchema with a
// PAN, processing code, amount, STAN, and an 8-byte track-2 blob.
func authMsg(t *testing.T) *Message {
	t.Helper()
	m := New(testSchema())
	for _, kv := range []struct {
		de int
		v  any
	}{
		{0, "0200"},
		{2, "4111111111111111"},
		{3, "000000"},
		{4, int64(2500)},
		{11, int64(42)},
		{35, []byte{0x41, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11}},
	} {
		if err := m.Set(kv.de, kv.v); err != nil {
			t.Fatalf("set DE %d: %v", kv.de, err)
		}
	}
	return m
}

func TestDumpMasksSensitiveByDefault(t *testing.T) {
	out := Dump(authMsg(t))

	// PAN keeps leading six + trailing four; the full PAN never appears.
	if !strings.Contains(out, "411111******1111") {
		t.Errorf("PAN not masked to leading6/trailing4:\n%s", out)
	}
	if strings.Contains(out, "4111111111111111") {
		t.Errorf("full PAN leaked into masked dump:\n%s", out)
	}
	// Track 2 is fully redacted, but its size still shows.
	if !strings.Contains(out, "[redacted 8B]") {
		t.Errorf("track-2 not redacted:\n%s", out)
	}
	// Non-sensitive scalars render plainly.
	if !strings.Contains(out, "Amount") || !strings.Contains(out, "2500") {
		t.Errorf("amount missing from dump:\n%s", out)
	}
	// Header carries the profile and MTI.
	if !strings.Contains(out, "profile test-ascii") || !strings.Contains(out, "MTI     : 0200") {
		t.Errorf("header missing profile/MTI:\n%s", out)
	}
}

func TestDumpUnmaskedShowsFull(t *testing.T) {
	out := Dump(authMsg(t), Unmasked())
	if !strings.Contains(out, "4111111111111111") {
		t.Errorf("Unmasked did not reveal the full PAN:\n%s", out)
	}
	if strings.Contains(out, "[redacted") {
		t.Errorf("Unmasked still redacted a field:\n%s", out)
	}
}

func TestDumpWithRawAddsHex(t *testing.T) {
	out := Dump(authMsg(t), Unmasked(), WithRaw())
	// PAN ASCII "4111..." -> raw hex of the digits.
	if !strings.Contains(out, "raw=34313131") {
		t.Errorf("WithRaw did not emit a raw-hex column:\n%s", out)
	}
}

// TestDumpWithRawHonoursMasking guards against the raw-hex column leaking the
// bytes of a masked field (which would defeat the redaction entirely).
func TestDumpWithRawHonoursMasking(t *testing.T) {
	out := Dump(authMsg(t), WithRaw()) // masked + raw
	if strings.Contains(out, "raw=34313131") {
		t.Errorf("raw column leaked masked PAN bytes:\n%s", out)
	}
	if !strings.Contains(out, "raw=[redacted]") {
		t.Errorf("masked field's raw column not redacted:\n%s", out)
	}
}

// emvSchema returns a parent schema with a BER-TLV composite at DE 55 carrying
// an application cryptogram (9F26) and an application PAN (5A).
func emvSchema(t *testing.T) *Schema {
	t.Helper()
	icc := NewSchema("icc").
		Tag("9F26", "Application Cryptogram", binBytes{}).
		Tag("5A", "Application PAN", asciiNum{}).
		MustBuild()
	return NewSchema("emv-test").
		MTI(asciiNum{}).
		Bitmap(BitmapSpec{Codec: binBitmap{}, Levels: 2}).
		Field(2, "PAN", asciiNum{}, llvarASCII{}, MaxLen(19)).
		Composite(55, "ICC Data", icc, llvarASCII{}).
		MustBuild()
}

func TestDescribeRecursesComposite(t *testing.T) {
	m := New(emvSchema(t))
	mustSet(t, m, 0, "0200")
	mustSet(t, m, 2, "4111111111111111")
	if err := m.SetS("55.9F26", []byte{0xA1, 0xB2, 0xC3, 0xD4}); err != nil {
		t.Fatalf("set 55.9F26: %v", err)
	}
	if err := m.SetS("55.5A", "5412345678901234"); err != nil {
		t.Fatalf("set 55.5A: %v", err)
	}

	out := Dump(m)
	// Composite parent line summarises its children.
	if !strings.Contains(out, "ICC Data") || !strings.Contains(out, "element(s)}") {
		t.Errorf("composite summary missing:\n%s", out)
	}
	// Children are addressed by dotted path and named from the sub-schema.
	if !strings.Contains(out, "55.9F26") || !strings.Contains(out, "Application Cryptogram") {
		t.Errorf("composite child 9F26 missing:\n%s", out)
	}
	if !strings.Contains(out, "55.9F26") || !strings.Contains(out, "A1B2C3D4") {
		t.Errorf("composite child cryptogram value missing:\n%s", out)
	}
	// A PAN buried in a TLV tag (5A) is masked just like a top-level PAN.
	if !strings.Contains(out, "541234******1234") {
		t.Errorf("TLV-embedded PAN (5A) not masked:\n%s", out)
	}
	if strings.Contains(out, "5412345678901234") {
		t.Errorf("TLV-embedded PAN leaked in full:\n%s", out)
	}
}

func TestMessageStringCompact(t *testing.T) {
	s := authMsg(t).String()
	if !strings.Contains(s, "test-ascii") || !strings.Contains(s, "mti=0200") {
		t.Errorf("compact String missing schema/mti: %q", s)
	}
	// The one-liner must never carry a field value (only the bitmap).
	if strings.Contains(s, "4111") {
		t.Errorf("compact String leaked a field value: %q", s)
	}
}

func TestLogValueMasksByDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("auth", "iso", authMsg(t))
	got := buf.String()

	if !strings.Contains(got, "411111******1111") {
		t.Errorf("slog LogValue did not mask PAN:\n%s", got)
	}
	if strings.Contains(got, "4111111111111111") {
		t.Errorf("slog LogValue leaked full PAN:\n%s", got)
	}
	if !strings.Contains(got, "iso.schema=test-ascii") || !strings.Contains(got, "iso.mti=0200") {
		t.Errorf("slog group missing schema/mti attrs:\n%s", got)
	}
	if !strings.Contains(got, "redacted 8B") {
		t.Errorf("slog LogValue did not redact track-2:\n%s", got)
	}
}

func TestLogValuerUnmasked(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("auth", "iso", LogValuer(authMsg(t), Unmasked()))
	if !strings.Contains(buf.String(), "4111111111111111") {
		t.Errorf("LogValuer(Unmasked) did not reveal PAN:\n%s", buf.String())
	}
}

func TestDescribeColor(t *testing.T) {
	m := authMsg(t)
	if strings.Contains(Dump(m), "\x1b[") {
		t.Error("default Dump should emit no ANSI codes")
	}
	colored := Dump(m, WithColor())
	if !strings.Contains(colored, "\x1b[") {
		t.Error("WithColor should emit ANSI codes")
	}
	// Colour must not defeat masking.
	if strings.Contains(colored, "4111111111111111") {
		t.Errorf("coloured dump leaked the full PAN:\n%s", colored)
	}
}

func mustSet(t *testing.T, m *Message, de int, v any) {
	t.Helper()
	if err := m.Set(de, v); err != nil {
		t.Fatalf("set DE %d: %v", de, err)
	}
}
