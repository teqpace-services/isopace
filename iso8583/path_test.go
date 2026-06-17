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

import "testing"

func TestParsePath(t *testing.T) {
	cases := []struct {
		in   string
		want []PathElem
	}{
		{"2", []PathElem{{N: 2}}},
		{"48.2.1", []PathElem{{N: 48}, {N: 2}, {N: 1}}},
		{"55.9F26", []PathElem{{N: 55}, {N: -1, Tag: "9F26"}}},
		{"55.9f26", []PathElem{{N: 55}, {N: -1, Tag: "9F26"}}}, // canonicalised upper
		{"0", []PathElem{{N: 0}}},
	}
	for _, c := range cases {
		p, err := ParsePath(c.in)
		if err != nil {
			t.Fatalf("ParsePath(%q) error: %v", c.in, err)
		}
		if p.Len() != len(c.want) {
			t.Fatalf("ParsePath(%q) len=%d want %d", c.in, p.Len(), len(c.want))
		}
		for i, w := range c.want {
			if p.At(i) != w {
				t.Errorf("ParsePath(%q)[%d]=%+v want %+v", c.in, i, p.At(i), w)
			}
		}
		if got := p.String(); got != canonical(c.in) {
			t.Errorf("ParsePath(%q).String()=%q want %q", c.in, got, canonical(c.in))
		}
	}
}

// canonical returns the expected round-trip form (hex tags uppercased).
func canonical(in string) string {
	p, _ := parsePathUncached(in)
	return p.String()
}

func TestParsePathErrors(t *testing.T) {
	bad := []string{"", "2.", ".2", "2..3", "1.2.3.4.5.6.7", "55.9G26", "5*"}
	for _, in := range bad {
		if _, err := ParsePath(in); err == nil {
			t.Errorf("ParsePath(%q) expected error, got nil", in)
		}
	}
}

func TestPathHeadTailPush(t *testing.T) {
	p := MustPath("55.9F26")
	if p.Head() != (PathElem{N: 55}) {
		t.Errorf("Head=%+v", p.Head())
	}
	tail := p.Tail()
	if tail.Len() != 1 || tail.At(0).Tag != "9F26" {
		t.Errorf("Tail=%+v", tail)
	}
	ext := DE(48).Push(PathElem{N: 2}).Push(PathElem{N: 1})
	if ext.String() != "48.2.1" {
		t.Errorf("Push chain = %q", ext.String())
	}
}

func TestPathComparableAsMapKey(t *testing.T) {
	m := map[FieldPath]int{}
	m[MustPath("55.9F26")] = 1
	m[DE(2)] = 2
	if m[MustPath("55.9f26")] != 1 { // canonical equality
		t.Errorf("map lookup by canonical path failed")
	}
	if m[DE(2)] != 2 {
		t.Errorf("map lookup by DE failed")
	}
}

func TestPathCacheConsistency(t *testing.T) {
	// Hitting the cache must return an equal path to a cold parse.
	cold, _ := parsePathUncached("48.2.1")
	for i := 0; i < 1000; i++ {
		got, err := ParsePath("48.2.1")
		if err != nil || got != cold {
			t.Fatalf("cache returned %v,%v want %v", got, err, cold)
		}
	}
}

func TestMustPathPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("MustPath bad input did not panic")
		}
	}()
	MustPath("nope!")
}
