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
	"log/slog"
	"sync"
)

// EventKind classifies a transaction lifecycle event recorded to a [Journal].
type EventKind int

const (
	// EventBegin marks the start of an exchange.
	EventBegin EventKind = iota
	// EventCommitted marks a successful commit pass.
	EventCommitted
	// EventAborted marks a rolled-back transaction.
	EventAborted
	// EventIdempotentHit marks a replayed response for a duplicate request.
	EventIdempotentHit
)

// String renders the event kind.
func (k EventKind) String() string {
	switch k {
	case EventBegin:
		return "begin"
	case EventCommitted:
		return "committed"
	case EventAborted:
		return "aborted"
	case EventIdempotentHit:
		return "idempotent-hit"
	default:
		return "unknown"
	}
}

// Event is a single journalled transaction lifecycle record.
type Event struct {
	Kind EventKind
	// Err is the abort cause for EventAborted, nil otherwise.
	Err error
}

// Journal records transaction lifecycle events. A durable implementation is the
// basis for store-and-forward recovery and audit: write before the effect so it
// can be replayed. Record runs inline on the flow goroutine; failures are logged
// by the flow but do not by themselves abort the transaction.
//
// Implementations must be safe for concurrent use across exchanges.
type Journal interface {
	Record(ctx context.Context, ex *Exchange, ev Event) error
}

// MemoryJournal is an in-memory audit trail of events, useful for tests and
// inspection. It retains every entry, so it is not for unbounded production use.
type MemoryJournal struct {
	mu      sync.Mutex
	entries []JournalEntry
}

// JournalEntry is one stored record: the exchange ID plus the event.
type JournalEntry struct {
	ID    string
	Event Event
}

// Record appends the event.
func (j *MemoryJournal) Record(_ context.Context, ex *Exchange, ev Event) error {
	j.mu.Lock()
	j.entries = append(j.entries, JournalEntry{ID: ex.ID, Event: ev})
	j.mu.Unlock()
	return nil
}

// Entries returns a snapshot of all recorded entries.
func (j *MemoryJournal) Entries() []JournalEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]JournalEntry(nil), j.entries...)
}

// SlogJournal writes lifecycle events to a structured logger.
type SlogJournal struct {
	Log *slog.Logger
}

// Record logs the event.
func (j SlogJournal) Record(ctx context.Context, ex *Exchange, ev Event) error {
	log := j.Log
	if log == nil {
		log = slog.Default()
	}
	attrs := []slog.Attr{slog.String("id", ex.ID), slog.String("event", ev.Kind.String())}
	if ev.Err != nil {
		attrs = append(attrs, slog.String("cause", ev.Err.Error()))
	}
	log.LogAttrs(ctx, slog.LevelInfo, "transaction", attrs...)
	return nil
}
