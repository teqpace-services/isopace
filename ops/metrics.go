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

package ops

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teqpace-services/isopace/runtime"
)

// Metrics is a metrics registry: counters, gauges, and summaries (count+sum,
// used for histograms and span durations), each broken out by label set. It
// implements runtime.Observer so instrumentation recorded through that facade is
// exposed here, and renders the Prometheus text exposition format via
// [Metrics.WriteText]. It is safe for concurrent use.
type Metrics struct {
	mu     sync.Mutex
	series map[string]*series
	kinds  map[string]metricKind // metric name -> its single kind
}

type metricKind int

const (
	kindCounter metricKind = iota
	kindGauge
	kindSummary
)

func (k metricKind) String() string {
	switch k {
	case kindGauge:
		return "gauge"
	case kindSummary:
		return "summary"
	default:
		return "counter"
	}
}

type series struct {
	name   string
	kind   metricKind
	labels []runtime.Attr
	value  float64 // counter total or gauge value
	count  uint64  // summary
	sum    float64 // summary
}

var _ runtime.Observer = (*Metrics)(nil)

// NewMetrics returns an empty metrics registry.
func NewMetrics() *Metrics {
	return &Metrics{series: map[string]*series{}, kinds: map[string]metricKind{}}
}

// Counter returns a counter instrument by name (runtime.Observer).
func (m *Metrics) Counter(name string) runtime.Counter { return counterHandle{m, name} }

// Histogram returns a summary instrument by name (runtime.Observer).
func (m *Metrics) Histogram(name string) runtime.Histogram { return histogramHandle{m, name} }

// Gauge returns a settable gauge instrument by name.
func (m *Metrics) Gauge(name string) *Gauge { return &Gauge{m, name} }

// StartSpan records the span's duration (seconds) into a summary named
// "<name>_duration_seconds" when the span ends (runtime.Observer).
func (m *Metrics) StartSpan(ctx context.Context, name string, attrs ...runtime.Attr) (context.Context, runtime.Span) {
	return ctx, &metricsSpan{m: m, name: name + "_duration_seconds", attrs: attrs, start: time.Now()}
}

type counterHandle struct {
	m    *Metrics
	name string
}

func (h counterHandle) Add(n int64, attrs ...runtime.Attr) {
	h.m.update(kindCounter, h.name, attrs, func(s *series) { s.value += float64(n) })
}

type histogramHandle struct {
	m    *Metrics
	name string
}

func (h histogramHandle) Observe(v float64, attrs ...runtime.Attr) {
	h.m.update(kindSummary, h.name, attrs, func(s *series) { s.count++; s.sum += v })
}

// Gauge is a settable instrument.
type Gauge struct {
	m    *Metrics
	name string
}

// Set sets the gauge value for the given labels.
func (g *Gauge) Set(v float64, attrs ...runtime.Attr) {
	g.m.update(kindGauge, g.name, attrs, func(s *series) { s.value = v })
}

// Add adds to the gauge value for the given labels.
func (g *Gauge) Add(v float64, attrs ...runtime.Attr) {
	g.m.update(kindGauge, g.name, attrs, func(s *series) { s.value += v })
}

type metricsSpan struct {
	m     *Metrics
	name  string
	attrs []runtime.Attr
	start time.Time
}

func (s *metricsSpan) End() {
	s.m.update(kindSummary, s.name, s.attrs, func(ser *series) {
		ser.count++
		ser.sum += time.Since(s.start).Seconds()
	})
}
func (s *metricsSpan) SetError(error)          {}
func (s *metricsSpan) SetAttr(...runtime.Attr) {}

func (m *Metrics) update(kind metricKind, name string, attrs []runtime.Attr, fn func(*series)) {
	sorted := sortedAttrs(attrs)
	key := seriesKey(name, sorted)
	m.mu.Lock()
	defer m.mu.Unlock()
	// A metric name is bound to one kind, so the Prometheus exposition stays
	// valid (a name cannot carry two # TYPE lines). Reusing a name as a
	// different kind is a deterministic instrumentation bug; panic like the
	// Prometheus client does rather than emit malformed output.
	if existing, ok := m.kinds[name]; ok {
		if existing != kind {
			panic(fmt.Sprintf("ops: metric %q used as %s but already registered as %s", name, kind, existing))
		}
	} else {
		m.kinds[name] = kind
	}
	s := m.series[key]
	if s == nil {
		s = &series{name: name, kind: kind, labels: sorted}
		m.series[key] = s
	}
	fn(s)
}

// WriteText writes the registry in the Prometheus text exposition format. Series
// are emitted grouped by metric name with a single # TYPE line each.
func (m *Metrics) WriteText(w io.Writer) error {
	m.mu.Lock()
	all := make([]*series, 0, len(m.series))
	for _, s := range m.series {
		all = append(all, &series{name: s.name, kind: s.kind, labels: s.labels, value: s.value, count: s.count, sum: s.sum})
	}
	m.mu.Unlock()

	sort.Slice(all, func(i, j int) bool {
		if all[i].name != all[j].name {
			return all[i].name < all[j].name
		}
		return renderLabels(all[i].labels) < renderLabels(all[j].labels)
	})

	var b strings.Builder
	lastName := ""
	for _, s := range all {
		if s.name != lastName {
			fmt.Fprintf(&b, "# TYPE %s %s\n", s.name, s.kind)
			lastName = s.name
		}
		lbl := renderLabels(s.labels)
		switch s.kind {
		case kindSummary:
			fmt.Fprintf(&b, "%s_count%s %d\n", s.name, lbl, s.count)
			fmt.Fprintf(&b, "%s_sum%s %s\n", s.name, lbl, formatFloat(s.sum))
		default:
			fmt.Fprintf(&b, "%s%s %s\n", s.name, lbl, formatFloat(s.value))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func sortedAttrs(attrs []runtime.Attr) []runtime.Attr {
	if len(attrs) == 0 {
		return nil
	}
	out := append([]runtime.Attr(nil), attrs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func seriesKey(name string, sorted []runtime.Attr) string {
	var b strings.Builder
	b.WriteString(name)
	for _, a := range sorted {
		b.WriteByte(0)
		b.WriteString(a.Key)
		b.WriteByte(1)
		fmt.Fprint(&b, a.Value)
	}
	return b.String()
}

func renderLabels(sorted []runtime.Attr) string {
	if len(sorted) == 0 {
		return ""
	}
	parts := make([]string, len(sorted))
	for i, a := range sorted {
		parts[i] = fmt.Sprintf("%s=%q", a.Key, fmt.Sprint(a.Value))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
