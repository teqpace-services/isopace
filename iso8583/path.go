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
	"errors"
	"strconv"
	"strings"
	"sync"
)

// maxPathDepth bounds a FieldPath so it stays an inline value type (no heap for
// typical depths): a top-level DE, a subfield chain (48.2.1), or a BER-TLV tag
// under a composite (55.9F26) all fit comfortably within six elements.
const maxPathDepth = 6

// PathElem is one step of a FieldPath: either a numeric DE / subfield index
// (N >= 0, Tag == "") or a BER-TLV tag in canonical uppercase hex (Tag != "",
// N == -1). Exactly one is set. PathElem is comparable so FieldPath is usable
// as a map key.
type PathElem struct {
	N   int32  // numeric DE / subfield index, or -1 when Tag is used
	Tag string // BER-TLV tag in canonical uppercase hex, e.g. "9F26"; "" otherwise
}

// IsTag reports whether the element addresses a BER-TLV tag rather than a
// numeric index.
func (e PathElem) IsTag() bool { return e.Tag != "" }

// String renders one element in the canonical grammar.
func (e PathElem) String() string {
	if e.Tag != "" {
		return e.Tag
	}
	return strconv.Itoa(int(e.N))
}

// FieldPath addresses a field at any depth under one uniform grammar used
// identically by the dynamic API, struct tags, validation errors, and JSON
// keys. It is an immutable, comparable value type.
//
//	ParsePath("2")        -> [{N:2}]
//	ParsePath("48.2.1")   -> [{N:48} {N:2} {N:1}]
//	ParsePath("55.9F26")  -> [{N:55} {Tag:"9F26"}]
type FieldPath struct {
	elems [maxPathDepth]PathElem // small inline array; no heap for typical depths
	n     uint8
}

// DE constructs a top-level numeric path: DE(2) == ParsePath("2").
func DE(n int) FieldPath {
	var p FieldPath
	p.elems[0] = PathElem{N: int32(n)}
	p.n = 1
	return p
}

// Len returns the number of path elements.
func (p FieldPath) Len() int { return int(p.n) }

// At returns the i-th element (0-based). It panics if i is out of range.
func (p FieldPath) At(i int) PathElem {
	if i < 0 || i >= int(p.n) {
		panic("iso8583: FieldPath index out of range")
	}
	return p.elems[i]
}

// Head returns the first element. Calling Head on an empty path panics.
func (p FieldPath) Head() PathElem { return p.At(0) }

// Tail returns the path without its first element.
func (p FieldPath) Tail() FieldPath {
	var t FieldPath
	if p.n <= 1 {
		return t
	}
	t.n = p.n - 1
	copy(t.elems[:t.n], p.elems[1:p.n])
	return t
}

// Push appends an element, returning the extended path. It panics if the path
// is already at maxPathDepth.
func (p FieldPath) Push(e PathElem) FieldPath {
	if int(p.n) >= maxPathDepth {
		panic("iso8583: FieldPath exceeds maxPathDepth")
	}
	p.elems[p.n] = e // p is a value copy; safe to mutate
	p.n++
	return p
}

// String renders the canonical textual form, e.g. "55.9F26".
func (p FieldPath) String() string {
	switch p.n {
	case 0:
		return ""
	case 1:
		return p.elems[0].String()
	}
	var b strings.Builder
	for i := uint8(0); i < p.n; i++ {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(p.elems[i].String())
	}
	return b.String()
}

// IsZero reports whether the path has no elements.
func (p FieldPath) IsZero() bool { return p.n == 0 }

// ErrEmptyPath is returned by ParsePath for an empty input string.
var ErrEmptyPath = errors.New("iso8583: empty field path")

// ErrPathTooDeep is returned when a path exceeds maxPathDepth elements.
var ErrPathTooDeep = errors.New("iso8583: field path exceeds maximum depth")

// pathCache memoises hot literals so repeated ParsePath("55.9F26") in routing
// code does not re-parse or re-allocate. It is a small FIFO-bounded cache: hot
// literals are stable, so simple bounded memoisation behaves like an LRU for
// the workload without the bookkeeping cost.
var pathCache = struct {
	sync.Mutex
	m     map[string]FieldPath
	ring  []string
	next  int
	limit int
}{m: make(map[string]FieldPath, 512), ring: make([]string, 512), limit: 512}

func cacheLookup(s string) (FieldPath, bool) {
	pathCache.Lock()
	p, ok := pathCache.m[s]
	pathCache.Unlock()
	return p, ok
}

func cacheStore(s string, p FieldPath) {
	pathCache.Lock()
	defer pathCache.Unlock()
	if _, ok := pathCache.m[s]; ok {
		return
	}
	if len(pathCache.m) >= pathCache.limit {
		if old := pathCache.ring[pathCache.next]; old != "" {
			delete(pathCache.m, old)
		}
	}
	pathCache.m[s] = p
	pathCache.ring[pathCache.next] = s
	pathCache.next = (pathCache.next + 1) % pathCache.limit
}

// ParsePath parses the textual grammar. Numeric elements are decimal; an
// element containing a hex letter (A–F) is a BER-TLV tag, canonicalised to
// uppercase. A small bounded cache memoises hot literals.
func ParsePath(s string) (FieldPath, error) {
	if s == "" {
		return FieldPath{}, ErrEmptyPath
	}
	if p, ok := cacheLookup(s); ok {
		return p, nil
	}
	p, err := parsePathUncached(s)
	if err != nil {
		return FieldPath{}, err
	}
	cacheStore(s, p)
	return p, nil
}

func parsePathUncached(s string) (FieldPath, error) {
	var p FieldPath
	start := 0
	for i := 0; i <= len(s); i++ {
		if i < len(s) && s[i] != '.' {
			continue
		}
		seg := s[start:i]
		start = i + 1
		if seg == "" {
			return FieldPath{}, &PathError{Input: s, Msg: "empty path element"}
		}
		if int(p.n) >= maxPathDepth {
			return FieldPath{}, ErrPathTooDeep
		}
		elem, err := parseElem(seg)
		if err != nil {
			return FieldPath{}, &PathError{Input: s, Msg: err.Error()}
		}
		p.elems[p.n] = elem
		p.n++
	}
	if p.n == 0 {
		return FieldPath{}, ErrEmptyPath
	}
	return p, nil
}

func parseElem(seg string) (PathElem, error) {
	hasHexLetter := false
	allDigits := true
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		switch {
		case c >= '0' && c <= '9':
			// digit; valid in both forms
		case (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f'):
			hasHexLetter = true
			allDigits = false
		default:
			return PathElem{}, errors.New("invalid character in path element")
		}
	}
	if hasHexLetter {
		// BER-TLV tag: canonical uppercase hex.
		return PathElem{N: -1, Tag: strings.ToUpper(seg)}, nil
	}
	if !allDigits {
		return PathElem{}, errors.New("invalid path element")
	}
	// A numeric element is a non-negative DE / subfield index; bound it to a
	// non-negative int32 so an overlong literal cannot wrap to a negative value.
	n, err := strconv.ParseInt(seg, 10, 32)
	if err != nil {
		return PathElem{}, errors.New("numeric element out of range")
	}
	return PathElem{N: int32(n)}, nil
}

// MustPath parses s and panics on error; intended for package-level constants.
func MustPath(s string) FieldPath {
	p, err := ParsePath(s)
	if err != nil {
		panic(err)
	}
	return p
}

// PathError reports a malformed FieldPath literal.
type PathError struct {
	Input string
	Msg   string
}

func (e *PathError) Error() string {
	return "iso8583: bad field path " + strconv.Quote(e.Input) + ": " + e.Msg
}
