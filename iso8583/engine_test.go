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
	"errors"
	"testing"
)

const validPAN = "4111111111111111" // Luhn-valid Visa test number

// buildAuth constructs a populated mutable message on the test schema.
func buildAuth(t *testing.T, s *Schema) *Message {
	t.Helper()
	m := New(s)
	must(t, m.Set(0, "0200"))
	must(t, m.Set(2, validPAN))
	must(t, m.Set(3, "0"))
	must(t, m.Set(4, NewDecimal(1099, 2)))
	must(t, m.Set(11, int64(123)))
	return m
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoundTrip(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)

	wire, err := c.Marshal(buildAuth(t, s), nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	m, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if mti, _ := Get[string](m, 0); mti != "0200" {
		t.Errorf("MTI = %q want 0200", mti)
	}
	if pan, _ := Get[string](m, 2); pan != validPAN {
		t.Errorf("PAN = %q want %q", pan, validPAN)
	}
	if stan, _ := Get[int64](m, 11); stan != 123 {
		t.Errorf("STAN = %d want 123", stan)
	}
	amt, err := Get[Decimal](m, 4)
	if err != nil || amt.String() != "10.99" {
		t.Errorf("Amount = %s, %v want 10.99", amt.String(), err)
	}
	if !m.Has(2) || m.Has(35) {
		t.Errorf("Has: DE2=%v DE35=%v", m.Has(2), m.Has(35))
	}

	// Bitmap reflects exactly the present data elements.
	var got []int
	m.Bitmap().Range(func(de int) bool { got = append(got, de); return true })
	if want := []int{2, 3, 4, 11}; !equalInts(got, want) {
		t.Errorf("present DEs = %v want %v", got, want)
	}
}

func TestMarshalCleanFastPath(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)
	wire, _ := c.Marshal(buildAuth(t, s), nil)

	m, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	out, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Unmodified message -> the original src is returned verbatim (aliased).
	if !sameUnderlying(out, wire) {
		t.Errorf("clean fast path did not return src verbatim (no aliasing)")
	}
}

func TestMarshalReEncodeMatchesOriginal(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)
	wire, _ := c.Marshal(buildAuth(t, s), nil)

	m, _ := c.Unmarshal(wire)
	edit := m.Clone()
	must(t, edit.Set(2, validPAN)) // re-set the same value -> forces owned re-encode

	out, err := c.Marshal(edit, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if sameUnderlying(out, wire) {
		t.Errorf("owned message must re-encode into a fresh buffer, not alias src")
	}
	if !bytes.Equal(out, wire) {
		t.Errorf("re-encoded wire differs from original:\n got %x\nwant %x", out, wire)
	}
}

func TestCopyOnWrite(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)
	wire, _ := c.Marshal(buildAuth(t, s), nil)

	base, _ := c.Unmarshal(wire)
	edit := base.Clone()
	must(t, edit.Set(39, "00"))

	if base.Has(39) {
		t.Errorf("mutation leaked into the original message (COW broken)")
	}
	if !edit.Has(39) {
		t.Errorf("Set did not take on the clone")
	}

	out, err := c.Marshal(edit, nil)
	if err != nil {
		t.Fatalf("Marshal edit: %v", err)
	}
	m2, _ := c.Unmarshal(out)
	if rc, _ := Get[string](m2, 39); rc != "00" {
		t.Errorf("DE39 = %q want 00", rc)
	}
	// Clean fields must have been copied straight from src, unchanged.
	if pan, _ := Get[string](m2, 2); pan != validPAN {
		t.Errorf("clean DE2 corrupted: %q", pan)
	}
}

func TestUnset(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)
	wire, _ := c.Marshal(buildAuth(t, s), nil)

	base, _ := c.Unmarshal(wire)
	edit := base.Clone()
	edit.Unset(11)
	if edit.Has(11) {
		t.Errorf("Unset(11) failed")
	}
	if !base.Has(11) {
		t.Errorf("Unset leaked into the original")
	}
	out, _ := c.Marshal(edit, nil)
	m2, _ := c.Unmarshal(out)
	if m2.Has(11) {
		t.Errorf("DE11 still present after Unset+round-trip")
	}
	if !m2.Has(4) {
		t.Errorf("unrelated DE4 dropped")
	}
}

func TestSealedRejectsMutation(t *testing.T) {
	s := testSchema()
	c := NewCodec(s, Immutable())
	wire, _ := NewCodec(s).Marshal(buildAuth(t, s), nil)

	m, _ := c.Unmarshal(wire)
	if err := m.Set(39, "00"); !errors.Is(err, errSealed) {
		t.Errorf("sealed Set error = %v want errSealed", err)
	}
	// Clone yields a mutable copy.
	if err := m.Clone().Set(39, "00"); err != nil {
		t.Errorf("clone of sealed message should be mutable: %v", err)
	}
}

func TestSecondaryBitmap(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)
	m := New(s)
	must(t, m.Set(0, "0800"))
	must(t, m.Set(70, "001")) // DE 70 lives in the secondary bitmap

	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m2.Bitmap().Width() != 128 {
		t.Errorf("expected secondary bitmap width 128, got %d", m2.Bitmap().Width())
	}
	if v, _ := Get[string](m2, 70); v != "001" {
		t.Errorf("DE70 = %q want 001", v)
	}
}

func TestZeroCopyBytesField(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)
	m := New(s)
	must(t, m.Set(0, "0200"))
	must(t, m.Set(35, []byte{0x12, 0x34, 0x56}))

	wire, _ := c.Marshal(m, nil)
	m2, _ := c.Unmarshal(wire)
	track, err := Get[[]byte](m2, 35)
	if err != nil {
		t.Fatalf("Get[[]byte]: %v", err)
	}
	if !bytes.Equal(track, []byte{0x12, 0x34, 0x56}) {
		t.Errorf("DE35 = % x want 12 34 56", track)
	}
}

func TestFieldsIterator(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)
	wire, _ := c.Marshal(buildAuth(t, s), nil)
	m, _ := c.Unmarshal(wire)

	var des []int
	for p, v := range m.Fields() {
		des = append(des, int(p.Head().N))
		if v.IsZero() {
			t.Errorf("Fields yielded a zero value at %s", p)
		}
	}
	if want := []int{2, 3, 4, 11}; !equalInts(des, want) {
		t.Errorf("Fields DEs = %v want %v", des, want)
	}
}

func TestValidateLuhn(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)

	good := buildAuth(t, s)
	if err := c.Validate(good); err != nil {
		t.Errorf("valid message reported violations: %v", err)
	}

	bad := buildAuth(t, s)
	must(t, bad.Set(2, "4111111111111112")) // breaks the Luhn check digit
	err := c.Validate(bad)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if len(ve.Violations) != 1 || ve.Violations[0].Rule != "luhn" {
		t.Errorf("violations = %+v want one luhn violation", ve.Violations)
	}
	if ve.Violations[0].Path.String() != "2" {
		t.Errorf("violation path = %q want 2", ve.Violations[0].Path.String())
	}
}

func TestValidateRequired(t *testing.T) {
	s := testSchema().Derive("req").Override(3, MustExist()).MustBuild()
	c := NewCodec(s)

	m := New(s)
	must(t, m.Set(0, "0200"))
	must(t, m.Set(2, validPAN))
	// DE3 omitted -> required violation.
	err := c.Validate(m)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	found := false
	for _, v := range ve.Violations {
		if v.Rule == "required" && v.Path.String() == "3" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected required violation for DE3, got %+v", ve.Violations)
	}
}

func TestSchemaBuildValidation(t *testing.T) {
	// Fixed field with no MaxLen must fail at build.
	_, err := NewSchema("bad").
		MTI(asciiNum{}).
		Bitmap(BitmapSpec{Codec: binBitmap{}, Levels: 2}).
		Field(2, "PAN", asciiNum{}, nil). // nil length, MaxLen 0
		Build()
	var se *SchemaError
	if !errors.As(err, &se) {
		t.Fatalf("expected *SchemaError, got %v", err)
	}
}

func TestUnknownDESetRejected(t *testing.T) {
	s := testSchema()
	m := New(s)
	if err := m.Set(99, "x"); !errors.Is(err, errUnknownDE) {
		t.Errorf("Set undefined DE error = %v want errUnknownDE", err)
	}
}

func sameUnderlying(a, b []byte) bool {
	return len(a) > 0 && len(b) > 0 && len(a) == len(b) && &a[0] == &b[0]
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
