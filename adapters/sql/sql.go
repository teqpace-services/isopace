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

// Package sql implements the Isopace store.Store interface over the standard
// library database/sql package. The caller supplies a configured *sql.DB (and
// therefore chooses the driver), so this adapter imports no SQL driver and adds
// no third-party dependency to a consumer's module graph beyond the driver they
// already chose.
package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/teqpace-services/isopace/store"
)

// Store implements store.Store.
var _ store.Store = (*Store)(nil)

// Dialect captures the few SQL details that differ across drivers: the
// positional placeholder syntax and the column type for opaque byte values.
type Dialect struct {
	Name        string
	Placeholder func(n int) string // 1-based positional placeholder, e.g. "?" or "$1"
	BlobType    string             // column type for opaque values
}

// Built-in dialects. Add others by constructing a Dialect.
var (
	SQLite   = Dialect{Name: "sqlite", Placeholder: qmark, BlobType: "BLOB"}
	MySQL    = Dialect{Name: "mysql", Placeholder: qmark, BlobType: "BLOB"}
	Postgres = Dialect{Name: "postgres", Placeholder: dollar, BlobType: "BYTEA"}
)

func qmark(int) string    { return "?" }
func dollar(n int) string { return "$" + strconv.Itoa(n) }

// validIdent guards the table name, which is interpolated into SQL (placeholders
// cannot parameterise identifiers).
var validIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Store is a collection/key/value store backed by a SQL table with columns
// (collection, k, v) and a composite primary key (collection, k).
type Store struct {
	db      *sql.DB
	dialect Dialect
	table   string
}

// Option configures a Store.
type Option func(*Store)

// WithTable sets the backing table name (default "isopace_kv"). The name must be
// a plain SQL identifier.
func WithTable(name string) Option { return func(s *Store) { s.table = name } }

// New returns a Store over db using the given Dialect. db must already be
// configured with a registered driver. Call EnsureSchema once to create the
// backing table if needed.
func New(db *sql.DB, d Dialect, opts ...Option) (*Store, error) {
	s := &Store{db: db, dialect: d, table: "isopace_kv"}
	for _, o := range opts {
		o(s)
	}
	if d.Placeholder == nil {
		return nil, errors.New("sqlstore: dialect has no Placeholder func")
	}
	if !validIdent.MatchString(s.table) {
		return nil, fmt.Errorf("sqlstore: invalid table name %q", s.table)
	}
	return s, nil
}

func (s *Store) ph(n int) string { return s.dialect.Placeholder(n) }

// EnsureSchema creates the backing table if it does not already exist.
func (s *Store) EnsureSchema(ctx context.Context) error {
	q := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (collection TEXT NOT NULL, k TEXT NOT NULL, v %s, PRIMARY KEY (collection, k))`,
		s.table, s.dialect.BlobType)
	_, err := s.db.ExecContext(ctx, q)
	return err
}

// Get returns the value stored under (collection, key), or store.ErrNotFound.
func (s *Store) Get(ctx context.Context, collection, key string) ([]byte, error) {
	q := fmt.Sprintf(`SELECT v FROM %s WHERE collection = %s AND k = %s`, s.table, s.ph(1), s.ph(2))
	var v []byte
	switch err := s.db.QueryRowContext(ctx, q, collection, key).Scan(&v); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, store.ErrNotFound
	case err != nil:
		return nil, err
	}
	return v, nil
}

// Put stores value under (collection, key), inserting or updating as needed.
func (s *Store) Put(ctx context.Context, collection, key string, value []byte) error {
	// Portable upsert: UPDATE first, INSERT only if nothing was updated. This
	// avoids dialect-specific ON CONFLICT / ON DUPLICATE KEY syntax.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	upd := fmt.Sprintf(`UPDATE %s SET v = %s WHERE collection = %s AND k = %s`, s.table, s.ph(1), s.ph(2), s.ph(3))
	res, err := tx.ExecContext(ctx, upd, value, collection, key)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		ins := fmt.Sprintf(`INSERT INTO %s (collection, k, v) VALUES (%s, %s, %s)`, s.table, s.ph(1), s.ph(2), s.ph(3))
		if _, err := tx.ExecContext(ctx, ins, collection, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Delete removes (collection, key). Deleting a missing key is not an error.
func (s *Store) Delete(ctx context.Context, collection, key string) error {
	q := fmt.Sprintf(`DELETE FROM %s WHERE collection = %s AND k = %s`, s.table, s.ph(1), s.ph(2))
	_, err := s.db.ExecContext(ctx, q, collection, key)
	return err
}

// List returns the keys in a collection in ascending order.
func (s *Store) List(ctx context.Context, collection string) ([]string, error) {
	q := fmt.Sprintf(`SELECT k FROM %s WHERE collection = %s ORDER BY k`, s.table, s.ph(1))
	rows, err := s.db.QueryContext(ctx, q, collection)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Close closes the underlying *sql.DB.
func (s *Store) Close() error { return s.db.Close() }
