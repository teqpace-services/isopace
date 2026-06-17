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

import (
	"sync"

	"github.com/teqpace-services/isopace/iso8583"
)

// KeyFunc derives an idempotency key from an exchange and reports whether the
// transaction is idempotent at all (false skips idempotency for that exchange).
// A typical key joins the retrieval-reference / STAN with the terminal so a
// retransmitted request is recognised as a duplicate.
type KeyFunc func(ex *Exchange) (key string, ok bool)

// IdempotencyStore remembers the response produced for an idempotency key so a
// duplicate request can be answered with the original response instead of being
// reprocessed. Implementations must be safe for concurrent use.
type IdempotencyStore interface {
	// Lookup returns the stored response for key, if present.
	Lookup(key string) (resp *iso8583.Message, found bool)
	// Store records the response for key.
	Store(key string, resp *iso8583.Message)
}

// MemoryStore is an in-memory IdempotencyStore. It clones messages on store and
// lookup so cached responses cannot be mutated through the store. It retains
// every key, so production deployments should bound it (TTL/eviction) or back it
// with a shared store; this implementation is for single-process use and tests.
type MemoryStore struct {
	mu   sync.Mutex
	resp map[string]*iso8583.Message
}

// NewMemoryStore builds an empty in-memory idempotency store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{resp: map[string]*iso8583.Message{}}
}

// Lookup returns a clone of the stored response for key.
func (s *MemoryStore) Lookup(key string) (*iso8583.Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.resp[key]
	if !ok || m == nil {
		return nil, ok
	}
	return m.Clone(), true
}

// Store records a clone of resp under key. A nil resp records the key with no
// payload, so a duplicate is still recognised (and answered with a nil response).
func (s *MemoryStore) Store(key string, resp *iso8583.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if resp != nil {
		resp = resp.Clone()
	}
	s.resp[key] = resp
}
