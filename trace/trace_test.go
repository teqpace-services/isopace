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

package trace_test

import (
	"context"
	"strings"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/trace"
)

func msg(t *testing.T) *iso8583.Message {
	t.Helper()
	m := iso8583.New(packager.ISO87A())
	for de, v := range map[int]any{0: "0200", 2: "4012345678909", 11: int64(7), 41: "TERM0001"} {
		if err := m.Set(de, v); err != nil {
			t.Fatalf("Set(%d): %v", de, err)
		}
	}
	return m
}

func TestTraceDescribe(t *testing.T) {
	tr := trace.New("isw-1")
	tr.Step("received", "from", "1.2.3.4", "bytes", 70)
	tr.Message("request", msg(t))
	tr.Step("route", "dest", "host")
	tr.Step("replied")

	out := tr.Describe()
	for _, want := range []string{"isw-1", "received", "from=1.2.3.4", "bytes=70", "request:", "0200", "route", "dest=host", "replied"} {
		if !strings.Contains(out, want) {
			t.Errorf("Describe missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "4012345678909") {
		t.Errorf("Describe leaked the full PAN (should be masked):\n%s", out)
	}
}

func TestTraceUnmasked(t *testing.T) {
	tr := trace.New("x")
	tr.Message("request", msg(t))
	if !strings.Contains(tr.Describe(iso8583.Unmasked()), "4012345678909") {
		t.Error("Unmasked() should reveal the full PAN")
	}
}

func TestTraceContext(t *testing.T) {
	if trace.From(context.Background()) != nil {
		t.Error("From(empty ctx) should be nil")
	}
	tr := trace.New("x")
	ctx := trace.With(context.Background(), tr)
	if got := trace.From(ctx); got != tr {
		t.Errorf("From(ctx) = %v want %v", got, tr)
	}
}

func TestTraceNilSafe(t *testing.T) {
	var tr *trace.Trace // never created
	tr.Step("noop", "k", "v")
	tr.Message("m", msg(t))
	tr.Fail(context.Canceled)
	if tr.Describe() != "" {
		t.Error("nil trace should describe to empty string")
	}
	// The nil from an empty context must be usable too.
	trace.From(context.Background()).Step("safe")
}

func TestTraceFail(t *testing.T) {
	tr := trace.New("x")
	tr.Fail(context.DeadlineExceeded)
	if !strings.Contains(tr.Describe(), "error") || !strings.Contains(tr.Describe(), context.DeadlineExceeded.Error()) {
		t.Errorf("Fail not rendered:\n%s", tr.Describe())
	}
}
