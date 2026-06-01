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
	"fmt"
	"strings"
	"time"

	"github.com/teqpace-services/isopace/iso8583"
)

// Exchange is the per-transaction state carried through a [Flow]. It holds the
// inbound Request and the Response being built, a free-form property bag for
// inter-stage communication, the abort state, and (when profiling is enabled)
// per-stage timings.
//
// An Exchange is owned by the single goroutine running its flow and is not safe
// for concurrent use.
type Exchange struct {
	// ID identifies the transaction in logs and journals (e.g. a STAN+terminal
	// key or a generated UUID). It is supplied by the caller.
	ID string
	// Request is the decoded inbound message. Stages treat it as immutable.
	Request *iso8583.Message
	// Response is the message being assembled for the caller; stages set and
	// refine it. It may be nil until a stage builds it.
	Response *iso8583.Message

	props   map[string]any
	timings []StageTiming
	err     error
	aborted bool
}

// StageTiming records how long one stage spent in a phase.
type StageTiming struct {
	Stage string
	Phase string // "prepare", "commit", or "abort"
	Dur   time.Duration
}

// NewExchange builds an exchange for request req identified by id.
func NewExchange(id string, req *iso8583.Message) *Exchange {
	return &Exchange{ID: id, Request: req, props: map[string]any{}}
}

// Set stores a value under key for later stages.
func (e *Exchange) Set(key string, v any) {
	if e.props == nil {
		e.props = map[string]any{}
	}
	e.props[key] = v
}

// Get returns the value stored under key and whether it was present.
func (e *Exchange) Get(key string) (any, bool) {
	v, ok := e.props[key]
	return v, ok
}

// Abort marks the transaction for rollback with the given cause. A stage may
// call this instead of (or before) returning an error — useful when it produces
// a decline response yet still wants the joined stages rolled back. A nil cause
// is recorded as [ErrAborted].
func (e *Exchange) Abort(cause error) {
	e.aborted = true
	if cause != nil {
		e.err = cause
	}
}

// Aborted reports whether the exchange has been marked for rollback.
func (e *Exchange) Aborted() bool { return e.aborted }

// Err returns the abort cause, or nil if the transaction has not aborted.
func (e *Exchange) Err() error { return e.err }

// Timings returns the recorded per-stage timings (empty unless profiling is on).
func (e *Exchange) Timings() []StageTiming {
	return append([]StageTiming(nil), e.timings...)
}

func (e *Exchange) record(stage, phase string, d time.Duration) {
	e.timings = append(e.timings, StageTiming{Stage: stage, Phase: phase, Dur: d})
}

// Profile renders the recorded timings as a single line, in order, with the
// total — handy for a per-transaction log field. Empty when profiling is off.
func (e *Exchange) Profile() string {
	if len(e.timings) == 0 {
		return ""
	}
	var b strings.Builder
	var total time.Duration
	for i, t := range e.timings {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%s/%s=%s", t.Stage, t.Phase, t.Dur)
		total += t.Dur
	}
	fmt.Fprintf(&b, " total=%s", total)
	return b.String()
}
