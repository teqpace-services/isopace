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

package flow_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/flow"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
)

// eventLog records stage events in order.
type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (e *eventLog) add(s string) {
	e.mu.Lock()
	e.events = append(e.events, s)
	e.mu.Unlock()
}

func (e *eventLog) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.events...)
}

// txStage is a full two-phase participant that logs each phase and can inject a
// custom Prepare or commit/abort errors.
type txStage struct {
	name      string
	log       *eventLog
	prepare   func(ctx context.Context, ex *flow.Exchange) (flow.Result, error)
	commitErr error
	abortErr  error
}

func (s *txStage) Name() string { return s.name }

func (s *txStage) Prepare(ctx context.Context, ex *flow.Exchange) (flow.Result, error) {
	s.log.add(s.name + ":prepare")
	if s.prepare != nil {
		return s.prepare(ctx, ex)
	}
	return flow.Continue(), nil
}

func (s *txStage) Commit(_ context.Context, _ *flow.Exchange) error {
	s.log.add(s.name + ":commit")
	return s.commitErr
}

func (s *txStage) Abort(_ context.Context, _ *flow.Exchange) error {
	s.log.add(s.name + ":abort")
	return s.abortErr
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newExchange(t *testing.T) *flow.Exchange {
	t.Helper()
	m := iso8583.New(packager.ISO87A())
	if err := m.Set(0, "0200"); err != nil {
		t.Fatal(err)
	}
	return flow.NewExchange("tx-1", m)
}

func TestFlowTwoPhaseCommitOrder(t *testing.T) {
	log := &eventLog{}
	f := flow.New(flow.WithLogger(quietLogger()))
	f.Group("main",
		&txStage{name: "a", log: log},
		&txStage{name: "b", log: log},
		&txStage{name: "c", log: log},
	)
	if err := f.Run(context.Background(), newExchange(t)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"a:prepare", "b:prepare", "c:prepare", "a:commit", "b:commit", "c:commit"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Errorf("order = %v want %v", got, want)
	}
}

func TestFlowAbortRollsBackInReverse(t *testing.T) {
	log := &eventLog{}
	boom := errors.New("downstream unreachable")
	f := flow.New(flow.WithLogger(quietLogger()))
	f.Group("main",
		&txStage{name: "a", log: log},
		&txStage{name: "b", log: log},
		&txStage{name: "c", log: log, prepare: func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
			log.add("c:prepare-fail")
			return flow.Continue(), boom
		}},
		&txStage{name: "d", log: log},
	)
	ex := newExchange(t)
	err := f.Run(context.Background(), ex)
	if !errors.Is(err, boom) {
		t.Fatalf("Run err = %v want boom", err)
	}
	if !ex.Aborted() {
		t.Error("exchange not marked aborted")
	}
	// a,b joined and roll back in reverse; c failed (not joined); d never runs.
	want := []string{"a:prepare", "b:prepare", "c:prepare", "c:prepare-fail", "b:abort", "a:abort"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Errorf("abort order = %v want %v", got, want)
	}
}

func TestFlowExchangeAbortDecline(t *testing.T) {
	log := &eventLog{}
	decline := errors.New("insufficient funds")
	f := flow.New(flow.WithLogger(quietLogger()))
	f.Group("main",
		&txStage{name: "auth", log: log},
		&txStage{name: "decline", log: log, prepare: func(_ context.Context, ex *flow.Exchange) (flow.Result, error) {
			ex.Abort(decline)
			return flow.Continue(), nil
		}},
		&txStage{name: "post", log: log},
	)
	ex := newExchange(t)
	err := f.Run(context.Background(), ex)
	if !errors.Is(err, decline) {
		t.Fatalf("Run err = %v want decline", err)
	}
	// auth + decline both joined (decline reserved work then aborted): both roll
	// back in reverse; post never runs.
	want := []string{"auth:prepare", "decline:prepare", "decline:abort", "auth:abort"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Errorf("decline order = %v want %v", got, want)
	}
}

func TestFlowRouting(t *testing.T) {
	log := &eventLog{}
	f := flow.New(flow.WithLogger(quietLogger()))
	f.Group("entry",
		&txStage{name: "parse", log: log},
		&txStage{name: "route", log: log, prepare: func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
			return flow.Route("issuerA"), nil
		}},
	)
	f.Group("issuerA",
		&txStage{name: "sendA", log: log},
		&txStage{name: "respA", log: log},
	)
	if err := f.Run(context.Background(), newExchange(t)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{
		"parse:prepare", "route:prepare", "sendA:prepare", "respA:prepare",
		"parse:commit", "route:commit", "sendA:commit", "respA:commit",
	}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Errorf("routing order = %v want %v", got, want)
	}
}

func TestFlowRouteUnknownGroupAborts(t *testing.T) {
	log := &eventLog{}
	f := flow.New(flow.WithLogger(quietLogger()))
	f.Group("entry",
		&txStage{name: "a", log: log, prepare: func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
			return flow.Route("ghost"), nil
		}},
	)
	err := f.Run(context.Background(), newExchange(t))
	if !errors.Is(err, flow.ErrUnknownGroup) {
		t.Fatalf("Run err = %v want ErrUnknownGroup", err)
	}
	// a joined then rolls back because routing failed.
	if got := log.snapshot(); !slices.Equal(got, []string{"a:prepare", "a:abort"}) {
		t.Errorf("events = %v", got)
	}
}

func TestFlowDoneShortCircuits(t *testing.T) {
	log := &eventLog{}
	f := flow.New(flow.WithLogger(quietLogger()))
	f.Group("main",
		&txStage{name: "a", log: log},
		&txStage{name: "done", log: log, prepare: func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
			return flow.Done(), nil
		}},
		&txStage{name: "never", log: log},
	)
	if err := f.Run(context.Background(), newExchange(t)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// 'a' joined and commits; 'done' does not join; 'never' is skipped.
	want := []string{"a:prepare", "done:prepare", "a:commit"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Errorf("done order = %v want %v", got, want)
	}
}

func TestFlowSkipDoesNotCommit(t *testing.T) {
	log := &eventLog{}
	f := flow.New(flow.WithLogger(quietLogger()))
	ran := false
	f.Group("main",
		flow.Func("readonly", func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
			log.add("readonly:prepare")
			ran = true
			return flow.Skip(), nil
		}),
		&txStage{name: "worker", log: log},
	)
	if err := f.Run(context.Background(), newExchange(t)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ran {
		t.Error("readonly stage did not run")
	}
	want := []string{"readonly:prepare", "worker:prepare", "worker:commit"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Errorf("skip order = %v want %v", got, want)
	}
}

func TestFlowRetrySucceedsAfterTransientFailures(t *testing.T) {
	var attempts atomic.Int32
	transient := errors.New("temporary")
	inner := flow.Func("flaky", func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
		if attempts.Add(1) < 3 {
			return flow.Continue(), transient
		}
		return flow.Continue(), nil
	})
	policy := flow.RetryPolicy{MaxAttempts: 5, Backoff: func(int) time.Duration { return time.Millisecond }}
	f := flow.New(flow.WithLogger(quietLogger()))
	f.Group("main", flow.Retry(inner, policy))
	if err := f.Run(context.Background(), newExchange(t)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d want 3", got)
	}
}

func TestFlowRetryExhaustsAndAborts(t *testing.T) {
	var attempts atomic.Int32
	fail := errors.New("always")
	inner := flow.Func("doomed", func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
		attempts.Add(1)
		return flow.Continue(), fail
	})
	policy := flow.RetryPolicy{MaxAttempts: 3, Backoff: func(int) time.Duration { return time.Millisecond }}
	f := flow.New(flow.WithLogger(quietLogger()))
	f.Group("main", flow.Retry(inner, policy))
	if err := f.Run(context.Background(), newExchange(t)); !errors.Is(err, fail) {
		t.Fatalf("Run err = %v want fail", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d want 3 (exhausted)", got)
	}
}

func TestFlowIdempotentReplay(t *testing.T) {
	log := &eventLog{}
	store := flow.NewMemoryStore()
	key := func(ex *flow.Exchange) (string, bool) { return ex.ID, true }

	var runs atomic.Int32
	f := flow.New(flow.WithLogger(quietLogger()), flow.WithIdempotency(store, key))
	f.Group("main",
		flow.Func("build", func(_ context.Context, ex *flow.Exchange) (flow.Result, error) {
			runs.Add(1)
			log.add("build")
			resp := iso8583.New(packager.ISO87A())
			_ = resp.Set(0, "0210")
			ex.Response = resp
			return flow.Continue(), nil
		}),
	)

	ex1 := newExchange(t)
	if err := f.Run(context.Background(), ex1); err != nil {
		t.Fatalf("Run1: %v", err)
	}
	// Same ID => duplicate => replay without re-running stages.
	ex2 := flow.NewExchange("tx-1", ex1.Request)
	if err := f.Run(context.Background(), ex2); err != nil {
		t.Fatalf("Run2: %v", err)
	}
	if got := runs.Load(); got != 1 {
		t.Errorf("stage ran %d times want 1 (second was a replay)", got)
	}
	if ex2.Response == nil {
		t.Error("replayed exchange has no response")
	}
	if mti, _ := iso8583.Get[string](ex2.Response, 0); mti != "0210" {
		t.Errorf("replayed MTI = %q want 0210", mti)
	}
}

func TestFlowJournalAndProfiler(t *testing.T) {
	j := &flow.MemoryJournal{}
	f := flow.New(flow.WithLogger(quietLogger()), flow.WithJournal(j), flow.WithProfiler(true))
	f.Group("main",
		&txStage{name: "a", log: &eventLog{}},
		flow.Func("b", func(_ context.Context, _ *flow.Exchange) (flow.Result, error) { return flow.Continue(), nil }),
	)
	ex := newExchange(t)
	if err := f.Run(context.Background(), ex); err != nil {
		t.Fatalf("Run: %v", err)
	}
	kinds := []flow.EventKind{}
	for _, e := range j.Entries() {
		kinds = append(kinds, e.Event.Kind)
	}
	if !slices.Equal(kinds, []flow.EventKind{flow.EventBegin, flow.EventCommitted}) {
		t.Errorf("journal kinds = %v", kinds)
	}
	if len(ex.Timings()) == 0 {
		t.Error("profiler recorded no timings")
	}
	if ex.Profile() == "" {
		t.Error("profile string empty with profiler on")
	}
}

func TestFlowMaxStagesGuardsRoutingLoop(t *testing.T) {
	f := flow.New(flow.WithLogger(quietLogger()), flow.WithMaxStages(10))
	f.Group("loop",
		flow.Func("spin", func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
			return flow.Route("loop"), nil
		}),
	)
	if err := f.Run(context.Background(), newExchange(t)); !errors.Is(err, flow.ErrTooManyStages) {
		t.Fatalf("Run err = %v want ErrTooManyStages", err)
	}
}

func TestFlowContextCancelAborts(t *testing.T) {
	f := flow.New(flow.WithLogger(quietLogger()))
	ctx, cancel := context.WithCancel(context.Background())
	f.Group("main",
		flow.Func("cancel", func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
			cancel()
			return flow.Continue(), nil
		}),
		flow.Func("after", func(_ context.Context, _ *flow.Exchange) (flow.Result, error) {
			t.Error("stage ran after context cancel")
			return flow.Continue(), nil
		}),
	)
	if err := f.Run(ctx, newExchange(t)); !errors.Is(err, context.Canceled) {
		t.Errorf("Run err = %v want context.Canceled", err)
	}
}

func TestFlowNoEntry(t *testing.T) {
	f := flow.New(flow.WithLogger(quietLogger()))
	if err := f.Run(context.Background(), newExchange(t)); !errors.Is(err, flow.ErrNoEntry) {
		t.Errorf("Run err = %v want ErrNoEntry", err)
	}
}
