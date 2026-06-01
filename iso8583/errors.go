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

// FieldError reports that a single field could not be decoded or typed. It
// carries the uniform FieldPath and, where known, the absolute byte offset into
// the source buffer (-1 when unknown) — invaluable when reading a hex dump.
type FieldError struct {
	Path   FieldPath // e.g. "55.9F26"
	Offset int       // absolute byte offset in the source buffer, or -1
	Name   string    // human field name, e.g. "Primary Account Number"
	Err    error     // underlying cause (luhn failed, truncated, length, type mismatch)
}

func (e *FieldError) Error() string {
	var b strings.Builder
	b.WriteString("field ")
	b.WriteString(e.Path.String())
	if e.Name != "" {
		b.WriteString(" (")
		b.WriteString(e.Name)
		b.WriteByte(')')
	}
	if e.Offset >= 0 {
		b.WriteString(" @byte ")
		b.WriteString(strconv.Itoa(e.Offset))
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Unwrap exposes the underlying cause for errors.Is / errors.As.
func (e *FieldError) Unwrap() error { return e.Err }

// Violation is a single semantic/validation failure produced by Codec.Validate.
type Violation struct {
	Path   FieldPath
	Offset int    // absolute byte offset, or -1
	Rule   string // "luhn", "len", "required", "currency", "mti", ...
	Msg    string
}

func (v Violation) String() string {
	var b strings.Builder
	b.WriteString(v.Path.String())
	if v.Offset >= 0 {
		b.WriteString(" @byte ")
		b.WriteString(strconv.Itoa(v.Offset))
	}
	b.WriteString(" [")
	b.WriteString(v.Rule)
	b.WriteString("]: ")
	b.WriteString(v.Msg)
	return b.String()
}

// asError adapts a Violation to the error interface for errors.Is / errors.As.
func (v Violation) asError() error { return violationErr{v} }

type violationErr struct{ v Violation }

func (e violationErr) Error() string { return e.v.String() }

// ValidationError aggregates all violations from one non-fail-fast Validate
// pass, so a declined message reports every problem at once with precise
// locations.
type ValidationError struct {
	Violations []Violation
}

func (e *ValidationError) Error() string {
	switch len(e.Violations) {
	case 0:
		return "iso8583: validation failed (no violations recorded)"
	case 1:
		return "iso8583: 1 validation violation: " + e.Violations[0].String()
	}
	var b strings.Builder
	b.WriteString("iso8583: ")
	b.WriteString(strconv.Itoa(len(e.Violations)))
	b.WriteString(" validation violations: ")
	for i, v := range e.Violations {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(v.String())
	}
	return b.String()
}

// Unwrap returns each Violation as an error so errors.Is / errors.As can match
// individual failures within the aggregate.
func (e *ValidationError) Unwrap() []error {
	if len(e.Violations) == 0 {
		return nil
	}
	errs := make([]error, len(e.Violations))
	for i, v := range e.Violations {
		errs[i] = v.asError()
	}
	return errs
}

// Sentinel causes shared across the engine.
var (
	errNotPresent  = errors.New("field not present")
	errNotText     = errors.New("value is not textual")
	errNotNumeric  = errors.New("value is not numeric")
	errWrongKind   = errors.New("value kind mismatch for requested type")
	errTruncated   = errors.New("wire buffer truncated")
	errUnknownDE   = errors.New("data element not defined by schema")
	errSealed      = errors.New("message is sealed; Clone before mutating")
	errBadLength   = errors.New("length prefix out of range")
	errNoComposite = errors.New("field is not a composite")
)
