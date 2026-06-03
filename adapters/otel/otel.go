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

// Package otel implements the Isopace runtime.Observer traces-and-metrics facade
// over OpenTelemetry. It lives in a separate module so the Isopace core never
// imports a telemetry SDK; wire it in at the edge:
//
//	host := runtime.NewHost(runtime.WithObserver(otel.Default()))
package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/teqpace-services/isopace/runtime"
)

// scopeName is the instrumentation scope reported to OpenTelemetry.
const scopeName = "github.com/teqpace-services/isopace"

// Observer implements runtime.Observer over OpenTelemetry providers.
type Observer struct {
	tracer trace.Tracer
	meter  metric.Meter
}

var _ runtime.Observer = (*Observer)(nil)

// New builds an Observer from explicit OpenTelemetry providers.
func New(tp trace.TracerProvider, mp metric.MeterProvider) *Observer {
	return &Observer{tracer: tp.Tracer(scopeName), meter: mp.Meter(scopeName)}
}

// Default builds an Observer from the globally-registered OpenTelemetry
// providers (otel.GetTracerProvider / otel.GetMeterProvider).
func Default() *Observer {
	return New(otel.GetTracerProvider(), otel.GetMeterProvider())
}

// StartSpan begins a span; the returned context carries it for propagation.
func (o *Observer) StartSpan(ctx context.Context, name string, attrs ...runtime.Attr) (context.Context, runtime.Span) {
	ctx, sp := o.tracer.Start(ctx, name, trace.WithAttributes(kvs(attrs)...))
	return ctx, &span{sp: sp}
}

// Counter returns a monotonic Int64 counter by name.
func (o *Observer) Counter(name string) runtime.Counter {
	c, err := o.meter.Int64Counter(name)
	if err != nil {
		return noopCounter{}
	}
	return &counter{c: c}
}

// Histogram returns a Float64 distribution instrument by name.
func (o *Observer) Histogram(name string) runtime.Histogram {
	h, err := o.meter.Float64Histogram(name)
	if err != nil {
		return noopHistogram{}
	}
	return &histogram{h: h}
}

type span struct{ sp trace.Span }

func (s *span) End() { s.sp.End() }

func (s *span) SetError(err error) {
	if err == nil {
		return
	}
	s.sp.RecordError(err)
	s.sp.SetStatus(codes.Error, err.Error())
}

func (s *span) SetAttr(attrs ...runtime.Attr) { s.sp.SetAttributes(kvs(attrs)...) }

type counter struct{ c metric.Int64Counter }

// Add records an increment. The runtime.Counter contract carries no context, so
// the background context is used; OpenTelemetry metric instruments do not depend
// on request context for aggregation.
func (c *counter) Add(n int64, attrs ...runtime.Attr) {
	c.c.Add(context.Background(), n, metric.WithAttributes(kvs(attrs)...))
}

type histogram struct{ h metric.Float64Histogram }

func (h *histogram) Observe(v float64, attrs ...runtime.Attr) {
	h.h.Record(context.Background(), v, metric.WithAttributes(kvs(attrs)...))
}

// noop instruments returned when the meter fails to construct one, so callers
// never need a nil check.
type noopCounter struct{}

func (noopCounter) Add(int64, ...runtime.Attr) {}

type noopHistogram struct{}

func (noopHistogram) Observe(float64, ...runtime.Attr) {}

// kvs converts Isopace attributes to OpenTelemetry key/values.
func kvs(attrs []runtime.Attr) []attribute.KeyValue {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, kv(a))
	}
	return out
}

func kv(a runtime.Attr) attribute.KeyValue {
	k := attribute.Key(a.Key)
	switch v := a.Value.(type) {
	case string:
		return k.String(v)
	case bool:
		return k.Bool(v)
	case int:
		return k.Int(v)
	case int64:
		return k.Int64(v)
	case float64:
		return k.Float64(v)
	default:
		return k.String(fmt.Sprintf("%v", v))
	}
}
