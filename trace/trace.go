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

// Package trace records the lifecycle of a single transaction — ordered,
// timestamped steps and message snapshots — so the whole request/response cycle
// can be described as one correlated unit. A [Trace] is carried in the request
// context ([With] / [From]), so every participant (a gateway, a transform hook,
// a routing function) annotates the same trace, and the complete story prints
// together via [Trace.Describe].
//
// Message snapshots are rendered with the iso8583 Describe renderer, so they are
// PCI-masked by default (pass iso8583.Unmasked() to [Trace.Describe] to override
// in a trusted context). All methods are safe on a nil *Trace, so call sites can
// write trace.From(ctx).Step(...) unconditionally whether or not tracing is on.
package trace

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/teqpace-services/isopace/iso8583"
)

// Trace is the accumulated lifecycle of one transaction.
type Trace struct {
	ID    string
	start time.Time

	mu      sync.Mutex
	entries []entry
}

type entryKind uint8

const (
	kindStep entryKind = iota
	kindMessage
	kindError
)

type entry struct {
	at     time.Duration
	kind   entryKind
	label  string
	detail string
	msg    *iso8583.Message
	err    error
}

// New starts a trace identified by id (typically a STAN+terminal key or a
// generated per-request id).
func New(id string) *Trace { return &Trace{ID: id, start: time.Now()} }

// Step records a timestamped event. Trailing arguments are rendered as
// space-separated key=value detail: Step("route", "dest", "isw").
func (t *Trace) Step(label string, kv ...any) {
	if t == nil {
		return
	}
	t.add(entry{kind: kindStep, label: label, detail: pairs(kv)})
}

// Message records a snapshot of m under title; it is described (and PCI-masked)
// when the trace is rendered.
func (t *Trace) Message(title string, m *iso8583.Message) {
	if t == nil || m == nil {
		return
	}
	t.add(entry{kind: kindMessage, label: title, msg: m})
}

// Fail records an error step.
func (t *Trace) Fail(err error) {
	if t == nil || err == nil {
		return
	}
	t.add(entry{kind: kindError, label: "error", err: err})
}

func (t *Trace) add(e entry) {
	e.at = time.Since(t.start)
	t.mu.Lock()
	t.entries = append(t.entries, e)
	t.mu.Unlock()
}

// Describe renders the whole lifecycle as a string. Options are passed to the
// message renderer (e.g. iso8583.Unmasked()).
func (t *Trace) Describe(opts ...iso8583.DescribeOption) string {
	var b strings.Builder
	t.WriteTo(&b, opts...)
	return b.String()
}

// WriteTo writes the lifecycle description to w.
func (t *Trace) WriteTo(w io.Writer, opts ...iso8583.DescribeOption) {
	if t == nil {
		return
	}
	t.mu.Lock()
	entries := append([]entry(nil), t.entries...)
	t.mu.Unlock()

	var total time.Duration
	if n := len(entries); n > 0 {
		total = entries[n-1].at
	}
	fmt.Fprintf(w, "━━ trace %s  (%s) ━━━━━━━━━━━━━━━━━━━━\n", t.ID, total.Round(time.Microsecond))
	for _, e := range entries {
		switch e.kind {
		case kindStep:
			fmt.Fprintf(w, "  %-10s %s", fmtAt(e.at), e.label)
			if e.detail != "" {
				fmt.Fprintf(w, "  %s", e.detail)
			}
			fmt.Fprintln(w)
		case kindError:
			fmt.Fprintf(w, "  %-10s error  %s\n", fmtAt(e.at), e.err)
		case kindMessage:
			fmt.Fprintf(w, "  %-10s %s:\n", fmtAt(e.at), e.label)
			dump := strings.TrimRight(iso8583.Dump(e.msg, opts...), "\n")
			for _, line := range strings.Split(dump, "\n") {
				fmt.Fprintf(w, "      %s\n", line)
			}
		}
	}
}

func fmtAt(d time.Duration) string { return "+" + d.Round(time.Microsecond).String() }

// pairs renders key=value detail from alternating arguments; a dangling final
// argument is appended on its own.
func pairs(kv []any) string {
	if len(kv) == 0 {
		return ""
	}
	var b strings.Builder
	i := 0
	for ; i+1 < len(kv); i += 2 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%v=%v", kv[i], kv[i+1])
	}
	if i < len(kv) {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%v", kv[i])
	}
	return b.String()
}

type ctxKey struct{}

// With returns a context carrying t, so downstream participants can annotate it.
func With(ctx context.Context, t *Trace) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// From returns the trace carried in ctx, or nil if none. The nil is usable: all
// *Trace methods are nil-safe, so trace.From(ctx).Step(...) is always valid.
func From(ctx context.Context) *Trace {
	t, _ := ctx.Value(ctxKey{}).(*Trace)
	return t
}
