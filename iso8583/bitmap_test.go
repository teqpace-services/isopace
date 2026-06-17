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
	"testing"
)

func TestBitmapSetIsSetClear(t *testing.T) {
	var b Bitmap
	for _, de := range []int{2, 3, 64, 65, 128, 129, 192} {
		b.Set(de)
	}
	for _, de := range []int{2, 3, 64, 65, 128, 129, 192} {
		if !b.IsSet(de) {
			t.Errorf("DE %d should be set", de)
		}
	}
	if b.IsSet(4) {
		t.Errorf("DE 4 should not be set")
	}
	b.Clear(65)
	if b.IsSet(65) {
		t.Errorf("DE 65 should be cleared")
	}
	// Out-of-range is a safe no-op / false.
	if b.IsSet(0) || b.IsSet(193) {
		t.Errorf("out-of-range DE reported set")
	}
}

func TestBitmapRangeAscendingExcludesContinuation(t *testing.T) {
	var b Bitmap
	// Set continuation bits plus real fields.
	b.Set(1)  // continuation
	b.Set(65) // continuation
	for _, de := range []int{2, 11, 70, 129} {
		b.Set(de)
	}
	var got []int
	b.Range(func(de int) bool {
		got = append(got, de)
		return true
	})
	want := []int{2, 11, 70, 129}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Range = %v want %v (continuation bits must be excluded)", got, want)
	}
	if b.Count() != 4 {
		t.Errorf("Count = %d want 4", b.Count())
	}
}

func TestBitmapWidth(t *testing.T) {
	var b Bitmap
	b.Set(2)
	if b.Width() != 64 {
		t.Errorf("primary-only width = %d want 64", b.Width())
	}
	b.Set(1) // secondary continuation
	if b.Width() != 128 {
		t.Errorf("secondary width = %d want 128", b.Width())
	}
	b.Set(65) // tertiary continuation
	if b.Width() != 192 {
		t.Errorf("tertiary width = %d want 192", b.Width())
	}
}

func TestBitmapRangeEarlyStop(t *testing.T) {
	var b Bitmap
	for _, de := range []int{2, 3, 4, 5} {
		b.Set(de)
	}
	count := 0
	b.Range(func(de int) bool {
		count++
		return count < 2 // stop after 2
	})
	if count != 2 {
		t.Errorf("early-stop yielded %d want 2", count)
	}
}
