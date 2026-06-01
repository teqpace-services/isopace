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
	"errors"
	"testing"
)

func TestDecimalString(t *testing.T) {
	cases := []struct {
		d    Decimal
		want string
	}{
		{NewDecimal(1099, 2), "10.99"},
		{NewDecimal(-1099, 2), "-10.99"},
		{NewDecimal(5, 3), "0.005"},
		{NewDecimal(-5, 3), "-0.005"},
		{NewDecimal(42, 0), "42"},
		{NewDecimal(0, 2), "0.00"},
		{NewDecimal(100, 2), "1.00"},
	}
	for _, c := range cases {
		if got := c.d.String(); got != c.want {
			t.Errorf("%+v.String()=%q want %q", c.d, got, c.want)
		}
	}
}

func TestDecimalRescale(t *testing.T) {
	d := NewDecimal(1099, 2)
	up, err := d.Rescale(4)
	if err != nil || up.Unscaled != 109900 || up.Scale != 4 {
		t.Fatalf("Rescale up = %+v, %v", up, err)
	}
	down, err := up.Rescale(2)
	if err != nil || down != d {
		t.Fatalf("Rescale down = %+v, %v", down, err)
	}
	if _, err := NewDecimal(1099, 2).Rescale(1); !errors.Is(err, ErrDecimalPrecision) {
		t.Errorf("expected precision loss error, got %v", err)
	}
}

func TestDecimalAdd(t *testing.T) {
	a := NewDecimal(1099, 2) // 10.99
	b := NewDecimal(5, 1)    // 0.5
	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}
	if sum.String() != "11.49" {
		t.Errorf("Add = %s want 11.49", sum.String())
	}
}

func TestDecimalAddOverflow(t *testing.T) {
	const maxInt64 = int64(^uint64(0) >> 1)
	a := NewDecimal(maxInt64, 0)
	if _, err := a.Add(NewDecimal(1, 0)); !errors.Is(err, ErrDecimalOverflow) {
		t.Errorf("expected overflow, got %v", err)
	}
}

func TestDecimalRescaleOverflow(t *testing.T) {
	const maxInt64 = int64(^uint64(0) >> 1)
	if _, err := NewDecimal(maxInt64, 0).Rescale(5); !errors.Is(err, ErrDecimalOverflow) {
		t.Errorf("expected overflow on rescale, got %v", err)
	}
}
