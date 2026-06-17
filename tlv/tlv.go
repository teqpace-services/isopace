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

// Package tlv decodes and encodes fixed-width tag-length-value streams: each
// element is a tag of TagWidth characters, a length of LenWidth decimal digits,
// then that many value characters, repeated to the end of the input. This is the
// shape used by several legacy acquirer parameter fields — for example CoralPay's
// terminal-parameter DE 62 (TagWidth 2, LenWidth 3: "03"+"015"+<15-char MID>,
// "05"+"003"+<currency>, …). It is distinct from BER-TLV (see fieldcodec) which
// uses encoded tag/length octets.
package tlv

import (
	"fmt"
	"strconv"
	"strings"
)

// Element is one decoded tag-length-value triple.
type Element struct {
	Tag   string
	Value string
}

// Decode parses s as a sequence of fixed-width tag-length-value elements. It
// returns an error if the stream is truncated, the length field is not numeric, or
// the widths are not positive.
func Decode(s string, tagWidth, lenWidth int) ([]Element, error) {
	if tagWidth <= 0 || lenWidth <= 0 {
		return nil, fmt.Errorf("tlv: tagWidth and lenWidth must be positive (got %d, %d)", tagWidth, lenWidth)
	}
	var out []Element
	for i := 0; i < len(s); {
		if i+tagWidth+lenWidth > len(s) {
			return nil, fmt.Errorf("tlv: truncated header at offset %d", i)
		}
		tag := s[i : i+tagWidth]
		lenStr := s[i+tagWidth : i+tagWidth+lenWidth]
		n, err := strconv.Atoi(lenStr)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("tlv: bad length %q for tag %q at offset %d", lenStr, tag, i)
		}
		start := i + tagWidth + lenWidth
		if start+n > len(s) {
			return nil, fmt.Errorf("tlv: value for tag %q overruns input (need %d, have %d)", tag, n, len(s)-start)
		}
		out = append(out, Element{Tag: tag, Value: s[start : start+n]})
		i = start + n
	}
	return out, nil
}

// Encode renders elements back to a fixed-width tag-length-value stream. Each tag
// must be exactly tagWidth characters and each value must fit in lenWidth digits.
func Encode(elems []Element, tagWidth, lenWidth int) (string, error) {
	if tagWidth <= 0 || lenWidth <= 0 {
		return "", fmt.Errorf("tlv: tagWidth and lenWidth must be positive (got %d, %d)", tagWidth, lenWidth)
	}
	max := 1
	for i := 0; i < lenWidth; i++ {
		max *= 10
	}
	var b strings.Builder
	for _, e := range elems {
		if len(e.Tag) != tagWidth {
			return "", fmt.Errorf("tlv: tag %q is not %d chars", e.Tag, tagWidth)
		}
		if len(e.Value) >= max {
			return "", fmt.Errorf("tlv: value for tag %q (len %d) does not fit in %d digits", e.Tag, len(e.Value), lenWidth)
		}
		fmt.Fprintf(&b, "%s%0*d%s", e.Tag, lenWidth, len(e.Value), e.Value)
	}
	return b.String(), nil
}

// Get returns the value of the first element with the given tag.
func Get(elems []Element, tag string) (string, bool) {
	for _, e := range elems {
		if e.Tag == tag {
			return e.Value, true
		}
	}
	return "", false
}
