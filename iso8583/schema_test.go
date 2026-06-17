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
	"slices"
	"testing"
)

// nameVal is a no-op validator that carries a name, so tests can assert
// validator identity and ordering after Derive/Override.
type nameVal struct{ n string }

func (nameVal) Validate(Value, *FieldDef) *Violation { return nil }

func namedValidator(n string) Validator { return nameVal{n} }

func validatorNames(s *Schema, de int) []string {
	d, ok := s.Field(de)
	if !ok {
		return nil
	}
	out := make([]string, len(d.Validate))
	for i, v := range d.Validate {
		out[i] = v.(nameVal).n
	}
	return out
}

// TestDeriveDoesNotAliasBaseValidators guards against the Derive/Override
// slice-aliasing bug: a derived overlay must not share its FieldDef.Validate
// backing array with the base schema. With the bug, appending a validator in one
// overlay scribbles into the base's spare capacity and into sibling overlays
// (e.g. ov1 would end up showing ov2's appended validator).
func TestDeriveDoesNotAliasBaseValidators(t *testing.T) {
	base := NewSchema("base").
		MTI(asciiNum{}).
		Bitmap(BitmapSpec{Codec: binBitmap{}, Levels: 2}).
		Field(2, "PAN", asciiNum{}, llvarASCII{}, MaxLen(19),
			Validate(namedValidator("v1"), namedValidator("v2"), namedValidator("v3"))).
		MustBuild()

	ov1 := base.Derive("ov1").Override(2, Validate(namedValidator("A"))).MustBuild()
	ov2 := base.Derive("ov2").Override(2, Validate(namedValidator("B"))).MustBuild()

	if got := validatorNames(base, 2); !slices.Equal(got, []string{"v1", "v2", "v3"}) {
		t.Errorf("base DE2 validators corrupted by overlay: got %v, want [v1 v2 v3]", got)
	}
	if got := validatorNames(ov1, 2); !slices.Equal(got, []string{"v1", "v2", "v3", "A"}) {
		t.Errorf("ov1 DE2 validators = %v, want [v1 v2 v3 A]", got)
	}
	if got := validatorNames(ov2, 2); !slices.Equal(got, []string{"v1", "v2", "v3", "B"}) {
		t.Errorf("ov2 DE2 validators = %v, want [v1 v2 v3 B] (aliasing would show A)", got)
	}
}
