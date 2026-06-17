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

package sql_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	sqlstore "github.com/teqpace-services/isopace/adapters/sql"
	"github.com/teqpace-services/isopace/store"
)

func TestDialectPlaceholders(t *testing.T) {
	if got := sqlstore.Postgres.Placeholder(1); got != "$1" {
		t.Errorf("Postgres placeholder = %q want $1", got)
	}
	if got := sqlstore.Postgres.Placeholder(3); got != "$3" {
		t.Errorf("Postgres placeholder = %q want $3", got)
	}
	if got := sqlstore.SQLite.Placeholder(2); got != "?" {
		t.Errorf("SQLite placeholder = %q want ?", got)
	}
}

func TestInvalidTableRejected(t *testing.T) {
	if _, err := sqlstore.New(nil, sqlstore.SQLite, sqlstore.WithTable("kv; DROP TABLE x")); err == nil {
		t.Fatal("expected an invalid table name to be rejected")
	}
}

func TestMissingDialectPlaceholder(t *testing.T) {
	if _, err := sqlstore.New(nil, sqlstore.Dialect{Name: "x"}); err == nil {
		t.Fatal("expected a dialect with no Placeholder to be rejected")
	}
}

// TestRoundTrip exercises the store against a real database. It is skipped unless
// ISOPACE_SQL_DRIVER and ISOPACE_SQL_DSN are set AND the chosen driver is
// imported into the test binary (add a blank import in your fork, e.g.
// _ "github.com/lib/pq"). CI wires this with a database service.
func TestRoundTrip(t *testing.T) {
	driver, dsn := os.Getenv("ISOPACE_SQL_DRIVER"), os.Getenv("ISOPACE_SQL_DSN")
	if driver == "" || dsn == "" {
		t.Skip("set ISOPACE_SQL_DRIVER and ISOPACE_SQL_DSN (and import the driver) to run")
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	dialect := sqlstore.SQLite
	if d := os.Getenv("ISOPACE_SQL_DIALECT"); d == "postgres" {
		dialect = sqlstore.Postgres
	} else if d == "mysql" {
		dialect = sqlstore.MySQL
	}
	st, err := sqlstore.New(db, dialect, sqlstore.WithTable("isopace_kv_test"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	if err := st.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	if _, err := st.Get(ctx, "c", "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get(missing) err = %v want ErrNotFound", err)
	}
	if err := st.Put(ctx, "c", "k1", []byte("v1")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := st.Put(ctx, "c", "k1", []byte("v1b")); err != nil { // upsert
		t.Fatalf("upsert: %v", err)
	}
	got, err := st.Get(ctx, "c", "k1")
	if err != nil || !bytes.Equal(got, []byte("v1b")) {
		t.Errorf("Get = %q, %v want v1b", got, err)
	}
	if err := st.Put(ctx, "c", "k2", []byte("v2")); err != nil {
		t.Fatalf("put k2: %v", err)
	}
	keys, err := st.List(ctx, "c")
	if err != nil || len(keys) != 2 {
		t.Errorf("List = %v, %v want 2 keys", keys, err)
	}
	if err := st.Delete(ctx, "c", "k1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.Get(ctx, "c", "k1"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("after delete, Get err = %v want ErrNotFound", err)
	}
}
