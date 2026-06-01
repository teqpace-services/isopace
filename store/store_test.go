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

package store_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/teqpace-services/isopace/store"
)

func TestMemStoreCRUD(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemStore()
	defer s.Close()

	if _, err := s.Get(ctx, "users", "alice"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get missing = %v want ErrNotFound", err)
	}
	if err := s.Put(ctx, "users", "alice", []byte("A")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, "users", "bob", []byte("B")); err != nil {
		t.Fatal(err)
	}
	v, err := s.Get(ctx, "users", "alice")
	if err != nil || string(v) != "A" {
		t.Errorf("Get = %q,%v want A", v, err)
	}
	keys, err := s.List(ctx, "users")
	if err != nil || !slices.Equal(keys, []string{"alice", "bob"}) {
		t.Errorf("List = %v,%v want [alice bob]", keys, err)
	}
	if err := s.Delete(ctx, "users", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "users", "alice"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get after delete = %v want ErrNotFound", err)
	}
	if err := s.Delete(ctx, "users", "absent"); err != nil {
		t.Errorf("delete missing key = %v want nil", err)
	}
	// Unknown collection lists empty, not error.
	if keys, err := s.List(ctx, "nope"); err != nil || len(keys) != 0 {
		t.Errorf("List unknown collection = %v,%v", keys, err)
	}
}

func TestMemStoreCopyIsolation(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemStore()
	in := []byte("data")
	if err := s.Put(ctx, "c", "k", in); err != nil {
		t.Fatal(err)
	}
	in[0] = 'X' // mutate caller's buffer after Put
	out, _ := s.Get(ctx, "c", "k")
	if string(out) != "data" {
		t.Errorf("Put did not copy: got %q", out)
	}
	out[0] = 'Y' // mutate returned slice
	out2, _ := s.Get(ctx, "c", "k")
	if string(out2) != "data" {
		t.Errorf("Get did not copy: got %q", out2)
	}
}

func TestStoreJSONHelpers(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemStore()
	type rec struct {
		N int    `json:"n"`
		S string `json:"s"`
	}
	want := rec{N: 7, S: "x"}
	if err := store.PutJSON(ctx, s, "recs", "1", want); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetJSON[rec](ctx, s, "recs", "1")
	if err != nil || got != want {
		t.Errorf("GetJSON = %+v,%v want %+v", got, err, want)
	}
	if _, err := store.GetJSON[rec](ctx, s, "recs", "absent"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetJSON missing = %v want ErrNotFound", err)
	}
}

func TestMemStoreConcurrent(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", n)
			if err := s.Put(ctx, "c", key, []byte(key)); err != nil {
				t.Errorf("Put: %v", err)
				return
			}
			if v, err := s.Get(ctx, "c", key); err != nil || string(v) != key {
				t.Errorf("Get(%s) = %q,%v", key, v, err)
			}
		}(i)
	}
	wg.Wait()
	if keys, _ := s.List(ctx, "c"); len(keys) != 50 {
		t.Errorf("expected 50 keys, got %d", len(keys))
	}
}
