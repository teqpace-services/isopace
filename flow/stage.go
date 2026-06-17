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

package flow

import "context"

// Stage is one participant in a [Flow]. Prepare does the stage's reversible work
// — validation, lookups, reservations — and votes on what happens next via the
// returned [Result]. Returning a non-nil error (or calling [Exchange.Abort])
// aborts the whole transaction.
//
// A Stage that needs to make durable effects or undo reserved work additionally
// implements [Committer] and/or [Aborter]; those run in the second phase only
// for stages that voted to continue (joined the transaction).
type Stage interface {
	Name() string
	Prepare(ctx context.Context, ex *Exchange) (Result, error)
}

// Committer is the commit half of two-phase processing: it runs after every
// stage has prepared successfully, in prepare order, to make effects durable.
type Committer interface {
	Commit(ctx context.Context, ex *Exchange) error
}

// Aborter is the rollback half: it runs when the transaction aborts, in reverse
// prepare order, to undo work reserved during Prepare.
type Aborter interface {
	Abort(ctx context.Context, ex *Exchange) error
}

// action is the disposition a stage's Prepare returns.
type action uint8

const (
	actContinue action = iota // proceed to the next stage and join the transaction
	actSkip                   // proceed but do not join commit/abort (read-only stage)
	actDone                   // stop the prepare pass; commit the stages joined so far
)

// Result is a stage's prepare-phase disposition. The zero Result is [Continue].
// Use the constructors below rather than building it directly.
type Result struct {
	action action
	groups []string
}

// Continue proceeds to the next stage and joins the transaction, so the stage's
// Commit (or Abort) runs in the second phase.
func Continue() Result { return Result{action: actContinue} }

// Skip proceeds to the next stage without joining the transaction — for a
// read-only stage that has no commit or rollback.
func Skip() Result { return Result{action: actSkip} }

// Route proceeds and appends the named groups' stages to the pipeline, so a
// stage can branch processing based on the exchange (e.g. select an issuer). The
// routing stage itself joins the transaction.
func Route(groups ...string) Result { return Result{action: actContinue, groups: groups} }

// Done stops the prepare pass and commits the stages that have joined so far —
// for a stage that has produced the final response and wants to short-circuit
// the remaining pipeline. The stage returning Done does not itself join the
// transaction, so its own Commit/Abort (if it implements them) will not run;
// return Continue instead if the terminal stage must participate in the commit
// phase.
func Done() Result { return Result{action: actDone} }

// funcStage adapts a function to Stage; see [Func].
type funcStage struct {
	name string
	fn   func(ctx context.Context, ex *Exchange) (Result, error)
}

func (s funcStage) Name() string { return s.name }

func (s funcStage) Prepare(ctx context.Context, ex *Exchange) (Result, error) {
	return s.fn(ctx, ex)
}

// Func builds a prepare-only Stage from a function — convenient for simple,
// read-only or response-building steps that need no commit/abort hooks.
func Func(name string, fn func(ctx context.Context, ex *Exchange) (Result, error)) Stage {
	return funcStage{name: name, fn: fn}
}
