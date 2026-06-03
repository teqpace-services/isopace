# Isopace SQL store adapter

A `store.Store` implementation backed by the standard library `database/sql`.

This is a **separate module** so the Isopace core stays stdlib-only: the adapter
imports only `database/sql`; **you** supply the driver, so no SQL driver enters
your module graph except the one you already chose.

```go
import (
    "database/sql"

    _ "github.com/lib/pq" // your driver
    sqlstore "github.com/teqpace-services/isopace/adapters/sql"
)

db, _ := sql.Open("postgres", dsn)
st, _ := sqlstore.New(db, sqlstore.Postgres)
_ = st.EnsureSchema(ctx) // creates the (collection, k, v) table if absent
// st satisfies store.Store
```

## Dialects

`New` takes a `Dialect` carrying the two things that differ across drivers — the
positional placeholder syntax and the opaque-value column type. Built-ins:
`SQLite`, `MySQL`, `Postgres`. Construct your own `Dialect` for others.

Writes use a portable UPDATE-then-INSERT upsert (no dialect-specific
`ON CONFLICT` / `ON DUPLICATE KEY`).

## Testing

Unit tests (dialects, validation) run with no database. The round-trip test is
skipped unless you set `ISOPACE_SQL_DRIVER`, `ISOPACE_SQL_DSN` (and
`ISOPACE_SQL_DIALECT`) **and** import the driver into the test binary.

> Follow-up: wire a Postgres service container into CI to run the round-trip on
> every build (see `ROADMAP-to-v1.md`, B2).
