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

// Package fieldcodec implements the pluggable VALUE codec catalog — the charset,
// numeric, amount and bitmap encodings that pair orthogonally with a
// lengthcodec length prefix to describe any ISO-8583 field. Codecs implement the
// interfaces declared in iso8583 (ARCHITECTURE.md §10 C1) and are populated into
// a Registry by the explicit DefaultRegistry() builder — no init() side effects.
//
// Every codec honours the canonical value form documented in
// iso8583/codec_iface.go: text/numeric decode to ASCII bytes, binary to the raw
// octets (zero copy where possible), amount to an exact Decimal.
package fieldcodec

import (
	"errors"

	"github.com/teqpace-services/isopace/iso8583"
)

// Errors shared by the value codecs.
var (
	ErrTooLong     = errors.New("fieldcodec: value exceeds fixed field width")
	ErrNonDigit    = errors.New("fieldcodec: non-digit byte in numeric value")
	ErrNegative    = errors.New("fieldcodec: negative value in unsigned field")
	ErrOddNibble   = errors.New("fieldcodec: malformed packed value")
	ErrBadHex      = errors.New("fieldcodec: malformed hex bitmap")
	ErrShortBitmap = errors.New("fieldcodec: truncated bitmap")
)

// digitCount returns the logical digit count for a packed (BCD) decode. The
// engine supplies units (the length prefix value, or MaxLen for a fixed field);
// when a caller cannot supply it (e.g. a BER-TLV envelope, which preserves only
// octet length), fall back to FieldDef.MaxLen. Deriving it from len(body)*2
// would be wrong for odd digit counts (the pad nibble would become a digit), so
// that heuristic is deliberately avoided. As a last resort an empty value
// decodes (Unpack with n==0), never corrupt digits.
func digitCount(units int, def *iso8583.FieldDef, _ []byte) int {
	if units > 0 {
		return units
	}
	if def != nil && def.MaxLen > 0 {
		return def.MaxLen
	}
	return 0
}

// isFixed reports whether a field is fixed-width: no length prefix and a
// positive MaxLen. A zero-MaxLen field with no prefix (e.g. a BER-TLV tag value,
// whose length comes from the TLV envelope) is treated as variable and emitted
// as-is without padding.
func isFixed(def *iso8583.FieldDef) bool { return def.Length == nil && def.MaxLen > 0 }

// padFixed returns b padded to def.MaxLen for a fixed field (variable fields are
// returned unchanged). It pads on the left when left is true, else on the right,
// and errors if b already exceeds the width.
func padFixed(b []byte, def *iso8583.FieldDef, pad byte, left bool) ([]byte, error) {
	if !isFixed(def) {
		return b, nil
	}
	switch {
	case len(b) > def.MaxLen:
		return nil, ErrTooLong
	case len(b) == def.MaxLen:
		return b, nil
	}
	out := make([]byte, def.MaxLen)
	gap := def.MaxLen - len(b)
	if left {
		for i := 0; i < gap; i++ {
			out[i] = pad
		}
		copy(out[gap:], b)
	} else {
		copy(out, b)
		for i := len(b); i < def.MaxLen; i++ {
			out[i] = pad
		}
	}
	return out, nil
}
