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

// Package trace records the lifecycle of a single transaction — ordered,
// timestamped steps and message snapshots — so the whole request/response cycle
// can be described as one correlated unit. A [Trace] is carried in the request
// context ([With] / [From]), so every participant (a gateway, a transform hook,
// a routing function) annotates the same trace, and the complete story prints
// together via [Trace.Describe].
//
// Message snapshots are rendered with the iso8583 Describe renderer, so they are
// PCI-masked by default (use [Unmasked] to override in a trusted context). All
// methods are safe on a nil *Trace, so call sites can write
// trace.From(ctx).Step(...) unconditionally whether or not tracing is on.
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

// DefaultTimeLayout is the per-step timestamp format used unless overridden.
const DefaultTimeLayout = "2006-01-02 15:04:05.000000"

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
	when   time.Time
	kind   entryKind
	label  string
	detail string
	msg    *iso8583.Message
	err    error
}

// New starts a trace identified by id (typically a STAN+terminal key or a
// generated per-request id).
func New(id string) *Trace { return &Trace{ID: id, start: time.Now()} }

// Step records a timestamped event. Trailing arguments render as space-separated
// key=value detail: Step("route", "dest", "isw").
func (t *Trace) Step(label string, kv ...any) {
	if t == nil {
		return
	}
	t.add(entry{kind: kindStep, label: label, detail: pairs(kv)})
}

// Message records a snapshot of m under title; it is described (PCI-masked) when
// the trace is rendered.
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
	e.when = time.Now()
	t.mu.Lock()
	t.entries = append(t.entries, e)
	t.mu.Unlock()
}

// Option configures how a trace renders.
type Option func(*renderConfig)

type renderConfig struct {
	timestamps bool
	layout     string
	color      bool
	msgOpts    []iso8583.DescribeOption
}

// NoTimestamps drops the per-step timestamp column.
func NoTimestamps() Option { return func(c *renderConfig) { c.timestamps = false } }

// WithColor turns on ANSI colour. It is off by default so output piped to a file
// or log pipeline stays clean; enable it for a terminal (see also [Color]).
func WithColor() Option { return func(c *renderConfig) { c.color = true } }

// Color sets ANSI colouring explicitly — handy with a runtime terminal check:
// trace.Color(isatty(os.Stdout)).
func Color(on bool) Option { return func(c *renderConfig) { c.color = on } }

// WithTimeLayout sets the timestamp layout (a time.Format reference layout).
func WithTimeLayout(layout string) Option { return func(c *renderConfig) { c.layout = layout } }

// Unmasked reveals sensitive fields (PAN, track, PIN) in message dumps. Use only
// in a trusted context.
func Unmasked() Option {
	return func(c *renderConfig) { c.msgOpts = append(c.msgOpts, iso8583.Unmasked()) }
}

// WithMessageOptions forwards arbitrary options to the message dumps.
func WithMessageOptions(opts ...iso8583.DescribeOption) Option {
	return func(c *renderConfig) { c.msgOpts = append(c.msgOpts, opts...) }
}

// Describe renders the whole lifecycle as a string.
func (t *Trace) Describe(opts ...Option) string {
	var b strings.Builder
	t.WriteTo(&b, opts...)
	return b.String()
}

// WriteTo writes the lifecycle description to w.
func (t *Trace) WriteTo(w io.Writer, opts ...Option) {
	if t == nil {
		return
	}
	cfg := renderConfig{timestamps: true, layout: DefaultTimeLayout}
	for _, o := range opts {
		o(&cfg)
	}

	t.mu.Lock()
	entries := append([]entry(nil), t.entries...)
	t.mu.Unlock()

	total := time.Duration(0)
	if n := len(entries); n > 0 {
		total = entries[n-1].when.Sub(t.start)
	}
	header := fmt.Sprintf("trace %s · total %s", t.ID, total.Round(time.Microsecond))
	fmt.Fprintf(w, "\n%s\n", cfg.paint(ansiBold, header))

	for _, e := range entries {
		switch e.kind {
		case kindMessage:
			fmt.Fprint(w, cfg.message(e))
		case kindError:
			fmt.Fprintf(w, "%s%s  %s\n", cfg.stamp(e.when), cfg.paint(ansiRed, "error"), cfg.paint(ansiRed, e.err.Error()))
		default:
			line := cfg.stamp(e.when) + cfg.paint(labelColor(e.label), fmt.Sprintf("%-14s", e.label))
			if e.detail != "" {
				line += "  " + e.detail
			}
			fmt.Fprintln(w, strings.TrimRight(line, " "))
		}
	}
}

// message renders a self-contained, titled message block (coloured by the
// iso8583 renderer when colour is on); no extra indentation.
func (c renderConfig) message(e entry) string {
	opts := append(e.msgTitle(), c.msgOpts...)
	if c.color {
		opts = append(opts, iso8583.WithColor())
	}
	return iso8583.Dump(e.msg, opts...)
}

// stamp renders the timestamp prefix (dim, with trailing spaces) or "" when off.
func (c renderConfig) stamp(when time.Time) string {
	if !c.timestamps {
		return ""
	}
	return c.paint(ansiDim, when.Format(c.layout)) + "  "
}

// ANSI styles used when colour is enabled.
const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiRed   = "\x1b[31m"
	ansiGreen = "\x1b[32m"
	ansiCyan  = "\x1b[36m"
)

// labelColor picks a colour by step: the open/close of the cycle stand out in
// green, everything else is cyan.
func labelColor(label string) string {
	switch label {
	case "received", "replied":
		return ansiGreen
	default:
		return ansiCyan
	}
}

// paint wraps s in an ANSI code when colour is enabled.
func (c renderConfig) paint(code, s string) string {
	if !c.color || s == "" {
		return s
	}
	return code + s + ansiReset
}

// msgTitle gives the message dump its section title (the entry label).
func (e entry) msgTitle() []iso8583.DescribeOption {
	return []iso8583.DescribeOption{iso8583.WithTitle(e.label)}
}

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
