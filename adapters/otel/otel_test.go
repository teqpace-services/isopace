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

package otel_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	oteladapter "github.com/teqpace-services/isopace/adapters/otel"
	"github.com/teqpace-services/isopace/runtime"
)

var _ runtime.Observer = (*oteladapter.Observer)(nil)

func TestStartSpanRecordsNameAttributesAndError(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	obs := oteladapter.New(tp, noopmetric.NewMeterProvider())

	_, span := obs.StartSpan(context.Background(), "auth", runtime.A("mti", "0200"))
	span.SetAttr(runtime.A("rc", "00"))
	span.SetError(errors.New("declined"))
	span.End()

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	s := spans[0]
	if s.Name() != "auth" {
		t.Errorf("span name = %q want %q", s.Name(), "auth")
	}
	got := map[string]string{}
	for _, a := range s.Attributes() {
		got[string(a.Key)] = a.Value.AsString()
	}
	for k, want := range map[string]string{"mti": "0200", "rc": "00"} {
		if got[k] != want {
			t.Errorf("attribute %q = %q want %q (all=%v)", k, got[k], want, got)
		}
	}
	if s.Status().Code != codes.Error {
		t.Errorf("span status = %v want Error", s.Status().Code)
	}
}

func TestCounterAndHistogramDoNotPanic(t *testing.T) {
	obs := oteladapter.New(nooptrace.NewTracerProvider(), noopmetric.NewMeterProvider())
	obs.Counter("isopace_test_total").Add(1, runtime.A("k", "v"))
	obs.Histogram("isopace_test_latency_ms").Observe(1.5, runtime.A("k", "v"))
	_, sp := obs.StartSpan(context.Background(), "noop")
	sp.End()
}

func TestDefaultUsesGlobalProviders(t *testing.T) {
	if oteladapter.Default() == nil {
		t.Fatal("Default() returned nil")
	}
}
