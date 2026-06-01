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

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrNotFound is returned by Get for a missing collection/key.
var ErrNotFound = errors.New("store: not found")

// Store is a collection/key/value persistence interface. A collection is a
// namespace (like a table); within it, keys map to opaque byte values.
// Implementations must be safe for concurrent use.
type Store interface {
	Get(ctx context.Context, collection, key string) ([]byte, error)
	Put(ctx context.Context, collection, key string, value []byte) error
	Delete(ctx context.Context, collection, key string) error
	List(ctx context.Context, collection string) ([]string, error)
	Close() error
}

var _ Store = (*MemStore)(nil)

// MemStore is the in-process Store backend: a map of collections to key/value
// maps. Values are copied on Put and Get so a caller cannot mutate stored data
// through a returned slice.
type MemStore struct {
	mu   sync.RWMutex
	data map[string]map[string][]byte
}

// NewMemStore returns an empty in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{data: map[string]map[string][]byte{}}
}

// Get returns a copy of the value at collection/key, or ErrNotFound.
func (m *MemStore) Get(_ context.Context, collection, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.data[collection]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, collection, key)
	}
	v, ok := c[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, collection, key)
	}
	return append([]byte(nil), v...), nil
}

// Put stores a copy of value at collection/key.
func (m *MemStore) Put(_ context.Context, collection, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.data[collection]
	if !ok {
		c = map[string][]byte{}
		m.data[collection] = c
	}
	c[key] = append([]byte(nil), value...)
	return nil
}

// Delete removes collection/key. Deleting a missing key is not an error.
func (m *MemStore) Delete(_ context.Context, collection, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.data[collection]; ok {
		delete(c, key)
		if len(c) == 0 {
			delete(m.data, collection)
		}
	}
	return nil
}

// List returns the keys in a collection, sorted. An unknown collection yields an
// empty slice (not an error).
func (m *MemStore) List(_ context.Context, collection string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c := m.data[collection]
	out := make([]string, 0, len(c))
	for k := range c {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// Close releases the store (a no-op for MemStore).
func (m *MemStore) Close() error { return nil }

// PutJSON marshals v as JSON and stores it at collection/key.
func PutJSON[T any](ctx context.Context, s Store, collection, key string, v T) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("store: marshal %s/%s: %w", collection, key, err)
	}
	return s.Put(ctx, collection, key, data)
}

// GetJSON loads collection/key and unmarshals it into a T.
func GetJSON[T any](ctx context.Context, s Store, collection, key string) (T, error) {
	var v T
	data, err := s.Get(ctx, collection, key)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("store: unmarshal %s/%s: %w", collection, key, err)
	}
	return v, nil
}
