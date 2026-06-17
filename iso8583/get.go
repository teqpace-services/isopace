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
	"reflect"
	"time"
)

// FieldType is the closed set of Go types a field can decode to. Per
// ARCHITECTURE.md §10 C2 it is a union constraint (NOT a sealed interface): a
// built-in string cannot carry an unexported method, and a Go union may mix
// ~-approximation terms with exact named-struct terms. Dispatch inside Get is
// by the static type of T.
type FieldType interface {
	~string | ~int64 | ~uint64 | []byte | Decimal | Amount | time.Time | Bitmap | *Message
}

// Get decodes a top-level DE directly into a Go type with no interface boxing,
// returning a structured *FieldError on type mismatch or malformed bytes.
//
//	pan, _  := iso8583.Get[string](m, 2)
//	stan, _ := iso8583.Get[int64](m, 11)
//	amt, _  := iso8583.Get[iso8583.Decimal](m, 4)
func Get[T FieldType](m *Message, de int) (T, error) {
	v, ok := m.Get(de)
	return convert[T](v, ok, DE(de))
}

// GetP decodes the field at a pre-parsed path into T.
//
//	icc, _ := iso8583.GetP[[]byte](m, iso8583.MustPath("55.9F26"))
func GetP[T FieldType](m *Message, p FieldPath) (T, error) {
	v, ok := m.GetP(p)
	return convert[T](v, ok, p)
}

// GetS decodes the field at a textual path into T.
func GetS[T FieldType](m *Message, s string) (T, error) {
	p, err := ParsePath(s)
	if err != nil {
		var zero T
		return zero, err
	}
	v, ok := m.GetP(p)
	return convert[T](v, ok, p)
}

func convert[T FieldType](v Value, ok bool, p FieldPath) (T, error) {
	var zero T
	if !ok {
		return zero, &FieldError{Path: p, Offset: -1, Err: errNotPresent}
	}
	t := reflect.TypeFor[T]()

	// Exact named types first.
	switch t {
	case reflect.TypeFor[Decimal]():
		d, err := v.Decimal()
		if err != nil {
			return zero, withPath(err, p)
		}
		return reflect.ValueOf(d).Interface().(T), nil
	case reflect.TypeFor[Amount]():
		d, err := v.Decimal()
		if err != nil {
			return zero, withPath(err, p)
		}
		return reflect.ValueOf(Amount{Decimal: d}).Interface().(T), nil
	case reflect.TypeFor[time.Time]():
		tv, err := decodeTime(v)
		if err != nil {
			return zero, withPath(err, p)
		}
		return reflect.ValueOf(tv).Interface().(T), nil
	case reflect.TypeFor[Bitmap]():
		return zero, &FieldError{Path: p, Offset: v.off, Err: errWrongKind}
	case reflect.TypeFor[*Message]():
		sub, sok := v.Composite()
		if !sok {
			return zero, &FieldError{Path: p, Offset: v.off, Err: errNoComposite}
		}
		return reflect.ValueOf(sub).Interface().(T), nil
	}

	// Approximated scalar / slice terms (handles named types too).
	out := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.String:
		s, err := v.String()
		if err != nil {
			return zero, withPath(err, p)
		}
		out.SetString(s)
		return out.Interface().(T), nil
	case reflect.Int64:
		n, err := v.Int()
		if err != nil {
			return zero, withPath(err, p)
		}
		out.SetInt(n)
		return out.Interface().(T), nil
	case reflect.Uint64:
		u, err := v.Uint()
		if err != nil {
			return zero, withPath(err, p)
		}
		out.SetUint(u)
		return out.Interface().(T), nil
	case reflect.Slice: // []byte (zero-copy view)
		out.SetBytes(v.Bytes())
		return out.Interface().(T), nil
	}
	return zero, &FieldError{Path: p, Offset: v.off, Err: errWrongKind}
}

// withPath attaches the path to a *FieldError (or wraps a bare error) so the
// caller always learns which field failed.
func withPath(err error, p FieldPath) error {
	if fe, ok := err.(*FieldError); ok {
		if fe.Path.IsZero() {
			fe.Path = p
		}
		return fe
	}
	return &FieldError{Path: p, Offset: -1, Err: err}
}

// decodeTime parses a digit field into a time.Time using a layout inferred from
// its length. A format-tagged layout (struct binding) is honoured separately;
// here the common ISO-8583 widths are recognised.
func decodeTime(v Value) (time.Time, error) {
	b := v.Bytes()
	var layout string
	switch len(b) {
	case 14:
		layout = "20060102150405" // YYYYMMDDhhmmss
	case 12:
		layout = "060102150405" // YYMMDDhhmmss
	case 10:
		layout = "0102150405" // MMDDhhmmss
	case 6:
		layout = "150405" // hhmmss
	case 4:
		layout = "0102" // MMDD
	default:
		return time.Time{}, &FieldError{Offset: v.off, Err: errWrongKind}
	}
	t, err := time.Parse(layout, string(b))
	if err != nil {
		return time.Time{}, &FieldError{Offset: v.off, Err: err}
	}
	return t, nil
}
