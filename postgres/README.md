# ledgo/postgres

PostgreSQL store for [ledgo](../README.md), on `database/sql` with the [pgx](https://github.com/jackc/pgx) driver. It is the reference implementation, and passes the [`ledgotest`](../ledgotest) conformance suite.

Also works with PostgreSQL-compatible services such as Aurora, AlloyDB, Neon and Supabase.

## Installation

```sh
go get github.com/izzyblues/ledgo/postgres@v0.0.1
```

## Usage

```go
db, err := sql.Open("pgx", "postgres://user:pass@localhost:5432/app")

store := postgres.New(db)
err = store.Migrate(ctx) // or copy schema.sql into your own migrations

ledger := ledgo.New(store) // each Commit runs in its own transaction
```

To commit postings atomically with your own changes, bind the store to your transaction:

```go
sqlTx, err := db.BeginTx(ctx, nil)
defer sqlTx.Rollback()

// ... your business changes on sqlTx ...
err = ledgo.New(postgres.InTx(sqlTx)).Commit(ctx, tx)

err = sqlTx.Commit() // postings and business changes commit together
```

If `Commit` fails, roll back `sqlTx`.

## Schema

[`schema.sql`](schema.sql) creates three tables: `ledgo_accounts`, `ledgo_transactions` and `ledgo_entries`. Amounts are `NUMERIC`, so any precision a currency allows fits.

## Concurrency

Commits lock only accounts with a balance limit, always in ID order, so concurrent commits cannot deadlock on the default `READ COMMITTED` isolation and need no retry loop. Accounts without a limit (clearing, FX, revenue) are insert-only and never locked.

## Development

```sh
docker compose up -d  # from the repository root
go test ./...
```
