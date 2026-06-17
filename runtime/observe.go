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

package runtime

import (
	"context"
	"log/slog"
)

// Observer is the traces-and-metrics facade. It is deliberately small and
// dependency-free so the core never imports a telemetry SDK: the default is
// [NopObserver], [SlogObserver] emits spans and measurements to a slog logger,
// and a real OpenTelemetry bridge is an external adapter that implements this
// interface. A nil Observer must never be dereferenced — use [Env.Observer],
// which substitutes a no-op.
type Observer interface {
	// StartSpan begins a unit of work. The returned context carries the span
	// for propagation; the caller must End the returned Span (typically via
	// defer). attrs annotate the span.
	StartSpan(ctx context.Context, name string, attrs ...Attr) (context.Context, Span)
	// Counter returns a monotonically increasing instrument by name.
	Counter(name string) Counter
	// Histogram returns a distribution instrument by name (e.g. latencies).
	Histogram(name string) Histogram
}

// Span is an in-flight unit of work. End records its completion; it must be
// called exactly once. SetError marks the span as failed.
type Span interface {
	End()
	SetError(err error)
	SetAttr(attrs ...Attr)
}

// Counter accumulates a monotonic total.
type Counter interface {
	Add(n int64, attrs ...Attr)
}

// Histogram records a distribution of observed values.
type Histogram interface {
	Observe(v float64, attrs ...Attr)
}

// Attr is a single key/value annotation on a span or measurement.
type Attr struct {
	Key   string
	Value any
}

// A builds an Attr; it reads well inline: obs.Counter("tx").Add(1, runtime.A("mti", "0200")).
func A(key string, value any) Attr { return Attr{Key: key, Value: value} }

// NopObserver discards everything. It is the zero-cost default so instrumented
// code can call the facade unconditionally.
type NopObserver struct{}

func (NopObserver) StartSpan(ctx context.Context, _ string, _ ...Attr) (context.Context, Span) {
	return ctx, nopSpan{}
}
func (NopObserver) Counter(string) Counter     { return nopInstrument{} }
func (NopObserver) Histogram(string) Histogram { return nopInstrument{} }

type nopSpan struct{}

func (nopSpan) End()            {}
func (nopSpan) SetError(error)  {}
func (nopSpan) SetAttr(...Attr) {}

type nopInstrument struct{}

func (nopInstrument) Add(int64, ...Attr)       {}
func (nopInstrument) Observe(float64, ...Attr) {}

// SlogObserver implements Observer by emitting spans and measurements as
// structured log records. It is a useful default in development and a faithful
// stand-in until a real metrics/tracing backend is wired in. It is safe for
// concurrent use (slog.Logger is).
type SlogObserver struct {
	Log *slog.Logger
}

func (o SlogObserver) logger() *slog.Logger {
	if o.Log != nil {
		return o.Log
	}
	return slog.Default()
}

// StartSpan logs span.start immediately and returns a span that logs span.end
// when ended; slog timestamps both records, so duration is recoverable from the
// log without this facade tracking wall-clock itself.
func (o SlogObserver) StartSpan(ctx context.Context, name string, attrs ...Attr) (context.Context, Span) {
	o.logger().LogAttrs(ctx, slog.LevelDebug, "span.start", append([]slog.Attr{slog.String("span", name)}, slogAttrs(attrs)...)...)
	return ctx, &slogSpan{log: o.logger(), name: name, ctx: ctx}
}

func (o SlogObserver) Counter(name string) Counter {
	return slogInstrument{log: o.logger(), name: name, kind: "counter"}
}

func (o SlogObserver) Histogram(name string) Histogram {
	return slogInstrument{log: o.logger(), name: name, kind: "histogram"}
}

type slogSpan struct {
	log  *slog.Logger
	name string
	ctx  context.Context
	err  error
}

func (s *slogSpan) End() {
	lvl := slog.LevelDebug
	attrs := []slog.Attr{slog.String("span", s.name)}
	if s.err != nil {
		lvl = slog.LevelError
		attrs = append(attrs, slog.String("error", s.err.Error()))
	}
	s.log.LogAttrs(s.ctx, lvl, "span.end", attrs...)
}

func (s *slogSpan) SetError(err error) { s.err = err }

func (s *slogSpan) SetAttr(attrs ...Attr) {
	s.log.LogAttrs(s.ctx, slog.LevelDebug, "span.attr", append([]slog.Attr{slog.String("span", s.name)}, slogAttrs(attrs)...)...)
}

type slogInstrument struct {
	log  *slog.Logger
	name string
	kind string
}

func (m slogInstrument) Add(n int64, attrs ...Attr) {
	m.log.LogAttrs(context.Background(), slog.LevelDebug, "metric",
		append([]slog.Attr{slog.String("name", m.name), slog.String("kind", m.kind), slog.Int64("delta", n)}, slogAttrs(attrs)...)...)
}

func (m slogInstrument) Observe(v float64, attrs ...Attr) {
	m.log.LogAttrs(context.Background(), slog.LevelDebug, "metric",
		append([]slog.Attr{slog.String("name", m.name), slog.String("kind", m.kind), slog.Float64("value", v)}, slogAttrs(attrs)...)...)
}

func slogAttrs(attrs []Attr) []slog.Attr {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = slog.Any(a.Key, a.Value)
	}
	return out
}
