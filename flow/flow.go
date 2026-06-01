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

package flow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Flow errors.
var (
	ErrNoEntry       = errors.New("flow: no entry group configured")
	ErrUnknownGroup  = errors.New("flow: unknown group")
	ErrTooManyStages = errors.New("flow: stage limit exceeded (routing loop?)")
	ErrAborted       = errors.New("flow: transaction aborted")
)

// Flow runs transactions through named groups of [Stage] values in two phases.
// Build it with [New] and [Flow.Group], then call [Flow.Run] per transaction; a
// single Flow is safe to run from many goroutines concurrently, provided its
// groups are not modified after the first Run.
type Flow struct {
	groups    map[string][]Stage
	entry     string
	log       *slog.Logger
	journal   Journal
	profile   bool
	maxStages int
	idemStore IdempotencyStore
	idemKey   KeyFunc
}

// Option configures a Flow.
type Option func(*Flow)

// WithLogger sets the flow logger (default slog.Default).
func WithLogger(l *slog.Logger) Option { return func(f *Flow) { f.log = l } }

// WithJournal records transaction lifecycle events to j.
func WithJournal(j Journal) Option { return func(f *Flow) { f.journal = j } }

// WithProfiler enables per-stage timing, recorded into each [Exchange].
func WithProfiler(on bool) Option { return func(f *Flow) { f.profile = on } }

// WithEntry names the group the flow starts from (default: the first group added
// with [Flow.Group]).
func WithEntry(group string) Option { return func(f *Flow) { f.entry = group } }

// WithMaxStages caps how many stages a single Run may execute, guarding against
// routing loops (default 1024).
func WithMaxStages(n int) Option { return func(f *Flow) { f.maxStages = n } }

// WithIdempotency makes Run replay the stored response for a duplicate request
// (as decided by key) instead of reprocessing it, and store the response of each
// committed transaction.
func WithIdempotency(store IdempotencyStore, key KeyFunc) Option {
	return func(f *Flow) { f.idemStore, f.idemKey = store, key }
}

// New builds a Flow.
func New(opts ...Option) *Flow {
	f := &Flow{
		groups:    map[string][]Stage{},
		log:       slog.Default(),
		maxStages: 1024,
	}
	for _, o := range opts {
		o(f)
	}
	if f.log == nil {
		f.log = slog.Default()
	}
	return f
}

// Group appends stages to the named group and returns the flow for chaining. The
// first group added becomes the entry group unless [WithEntry] set one.
func (f *Flow) Group(name string, stages ...Stage) *Flow {
	f.groups[name] = append(f.groups[name], stages...)
	if f.entry == "" {
		f.entry = name
	}
	return f
}

// Run drives ex through the pipeline. It returns nil on commit, or the abort
// cause if a stage failed or aborted (after running Abort on the joined stages),
// or a joined commit error if the commit phase failed.
func (f *Flow) Run(ctx context.Context, ex *Exchange) error {
	if f.entry == "" {
		return ErrNoEntry
	}
	entryStages, err := f.resolve(f.entry)
	if err != nil {
		return err
	}

	// Idempotent replay short-circuits a duplicate request.
	idemKey, idemActive := "", false
	if f.idemStore != nil && f.idemKey != nil {
		if k, ok := f.idemKey(ex); ok {
			idemKey, idemActive = k, true
			if resp, found := f.idemStore.Lookup(k); found {
				ex.Response = resp
				f.emit(ctx, ex, Event{Kind: EventIdempotentHit})
				return nil
			}
		}
	}

	f.emit(ctx, ex, Event{Kind: EventBegin})

	work := append([]Stage(nil), entryStages...)
	var joined []Stage
	var prepErr error

	for i := 0; i < len(work); i++ {
		if i >= f.maxStages {
			prepErr = ErrTooManyStages
			break
		}
		if cerr := ctx.Err(); cerr != nil {
			prepErr = cerr
			break
		}
		st := work[i]
		res, perr := f.prepare(ctx, ex, st)
		if perr != nil {
			prepErr = perr
			break
		}
		// Join before reacting to abort/done so that a stage which reserved
		// work and then called Exchange.Abort still gets its Abort rollback.
		if res.action == actContinue {
			joined = append(joined, st)
		}
		if ex.aborted {
			break
		}
		if res.action == actDone {
			break
		}
		if len(res.groups) > 0 {
			work, prepErr = f.appendGroups(work, res.groups)
			if prepErr != nil {
				break
			}
		}
	}

	// Abort path: a prepare error, an explicit Abort, or a routing/limit fault.
	if prepErr != nil || ex.aborted {
		cause := prepErr
		if cause == nil {
			cause = ex.err
		}
		if cause == nil {
			cause = ErrAborted
		}
		ex.aborted = true
		ex.err = cause
		f.abortJoined(ctx, ex, joined)
		f.emit(ctx, ex, Event{Kind: EventAborted, Err: cause})
		return cause
	}

	// Commit path.
	commitErr := f.commitJoined(ctx, ex, joined)
	f.emit(ctx, ex, Event{Kind: EventCommitted})
	if idemActive && commitErr == nil {
		f.idemStore.Store(idemKey, ex.Response)
	}
	return commitErr
}

func (f *Flow) resolve(name string) ([]Stage, error) {
	g, ok := f.groups[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownGroup, name)
	}
	return g, nil
}

func (f *Flow) appendGroups(work []Stage, names []string) ([]Stage, error) {
	for _, name := range names {
		g, err := f.resolve(name)
		if err != nil {
			return work, err
		}
		work = append(work, g...)
	}
	return work, nil
}

func (f *Flow) prepare(ctx context.Context, ex *Exchange, st Stage) (Result, error) {
	if !f.profile {
		return st.Prepare(ctx, ex)
	}
	start := time.Now()
	res, err := st.Prepare(ctx, ex)
	ex.record(st.Name(), "prepare", time.Since(start))
	return res, err
}

func (f *Flow) commitJoined(ctx context.Context, ex *Exchange, joined []Stage) error {
	var errs []error
	for _, st := range joined {
		c, ok := st.(Committer)
		if !ok {
			continue
		}
		start := time.Now()
		err := c.Commit(ctx, ex)
		if f.profile {
			ex.record(st.Name(), "commit", time.Since(start))
		}
		if err != nil {
			f.log.LogAttrs(ctx, slog.LevelError, "commit failed",
				slog.String("id", ex.ID), slog.String("stage", st.Name()), slog.String("error", err.Error()))
			errs = append(errs, fmt.Errorf("commit %q: %w", st.Name(), err))
		}
	}
	return errors.Join(errs...)
}

func (f *Flow) abortJoined(ctx context.Context, ex *Exchange, joined []Stage) {
	for i := len(joined) - 1; i >= 0; i-- {
		st := joined[i]
		a, ok := st.(Aborter)
		if !ok {
			continue
		}
		start := time.Now()
		err := a.Abort(ctx, ex)
		if f.profile {
			ex.record(st.Name(), "abort", time.Since(start))
		}
		if err != nil {
			f.log.LogAttrs(ctx, slog.LevelError, "abort failed",
				slog.String("id", ex.ID), slog.String("stage", st.Name()), slog.String("error", err.Error()))
		}
	}
}

func (f *Flow) emit(ctx context.Context, ex *Exchange, ev Event) {
	if f.journal == nil {
		return
	}
	if err := f.journal.Record(ctx, ex, ev); err != nil {
		f.log.LogAttrs(ctx, slog.LevelError, "journal write failed",
			slog.String("id", ex.ID), slog.String("event", ev.Kind.String()), slog.String("error", err.Error()))
	}
}
