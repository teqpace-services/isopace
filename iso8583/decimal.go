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
	"strconv"
	"strings"
)

// Decimal is an exact base-10 fixed-point number: value = Unscaled * 10^-Scale.
// No float64 ever touches a monetary value; all arithmetic is exact and reports
// overflow rather than silently losing precision.
type Decimal struct {
	Unscaled int64
	Scale    uint8
}

// Errors returned by Decimal arithmetic.
var (
	ErrDecimalOverflow  = errors.New("iso8583: decimal overflow")
	ErrDecimalPrecision = errors.New("iso8583: decimal rescale would lose precision")
	ErrScaleTooLarge    = errors.New("iso8583: decimal scale too large")
)

// maxScale bounds Scale so 10^Scale fits an int64 with room to spare.
const maxScale = 18

// pow10 returns 10^n as an int64, ok=false on overflow.
func pow10(n uint8) (int64, bool) {
	if n > maxScale {
		return 0, false
	}
	r := int64(1)
	for i := uint8(0); i < n; i++ {
		r *= 10
	}
	return r, true
}

// NewDecimal constructs a Decimal from an unscaled integer and a scale.
func NewDecimal(unscaled int64, scale uint8) Decimal {
	return Decimal{Unscaled: unscaled, Scale: scale}
}

// String renders the value in plain decimal notation, e.g. "10.99", "-0.005",
// or "42".
func (d Decimal) String() string {
	if d.Scale == 0 {
		return strconv.FormatInt(d.Unscaled, 10)
	}
	neg := d.Unscaled < 0
	// Use unsigned magnitude to avoid overflow on math.MinInt64.
	mag := uint64(d.Unscaled)
	if neg {
		mag = uint64(-(d.Unscaled + 1)) + 1
	}
	digits := strconv.FormatUint(mag, 10)
	scale := int(d.Scale)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	if len(digits) <= scale {
		b.WriteString("0.")
		b.WriteString(strings.Repeat("0", scale-len(digits)))
		b.WriteString(digits)
	} else {
		split := len(digits) - scale
		b.WriteString(digits[:split])
		b.WriteByte('.')
		b.WriteString(digits[split:])
	}
	return b.String()
}

// Rescale returns an equivalent Decimal at the given scale. Increasing the
// scale multiplies (reporting overflow); decreasing it requires the dropped
// digits to be zero (else ErrDecimalPrecision).
func (d Decimal) Rescale(scale uint8) (Decimal, error) {
	if scale > maxScale {
		return Decimal{}, ErrScaleTooLarge
	}
	if scale == d.Scale {
		return d, nil
	}
	if scale > d.Scale {
		factor, ok := pow10(scale - d.Scale)
		if !ok {
			return Decimal{}, ErrDecimalOverflow
		}
		hi, lo := mulCheck(d.Unscaled, factor)
		if !lo {
			return Decimal{}, ErrDecimalOverflow
		}
		return Decimal{Unscaled: hi, Scale: scale}, nil
	}
	factor, ok := pow10(d.Scale - scale)
	if !ok {
		return Decimal{}, ErrDecimalOverflow
	}
	if d.Unscaled%factor != 0 {
		return Decimal{}, ErrDecimalPrecision
	}
	return Decimal{Unscaled: d.Unscaled / factor, Scale: scale}, nil
}

// Add returns the exact sum, aligning scales to the larger of the two. It
// reports overflow rather than wrapping.
func (d Decimal) Add(o Decimal) (Decimal, error) {
	scale := d.Scale
	if o.Scale > scale {
		scale = o.Scale
	}
	a, err := d.Rescale(scale)
	if err != nil {
		return Decimal{}, err
	}
	b, err := o.Rescale(scale)
	if err != nil {
		return Decimal{}, err
	}
	sum, ok := addCheck(a.Unscaled, b.Unscaled)
	if !ok {
		return Decimal{}, ErrDecimalOverflow
	}
	return Decimal{Unscaled: sum, Scale: scale}, nil
}

// IsZero reports whether the value is exactly zero.
func (d Decimal) IsZero() bool { return d.Unscaled == 0 }

// mulCheck multiplies two int64 values, returning ok=false on overflow.
func mulCheck(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	r := a * b
	if r/b != a {
		return 0, false
	}
	return r, true
}

// addCheck adds two int64 values, returning ok=false on overflow.
func addCheck(a, b int64) (int64, bool) {
	r := a + b
	if (r > a) != (b > 0) {
		return 0, false
	}
	return r, true
}

// Amount is a Decimal with an ISO 4217 currency (numeric, e.g. "840" for USD),
// enabling cross-field validation against currency DEs (49/51).
type Amount struct {
	Decimal
	Currency string
}
