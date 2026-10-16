# ledgo/oracle

Oracle Database store for [ledgo](../README.md), on `database/sql` with the pure-Go [go-ora](https://github.com/sijms/go-ora) driver (no Oracle Instant Client or cgo needed). It passes the [`ledgotest`](../ledgotest) conformance suite.

Tested on Oracle Database 23ai. The schema avoids 23ai-only features, so 19c should work too.

## Installation

```sh
go get github.com/izzyblues/ledgo/oracle@v0.0.1
```

## Usage

```go
db, err := sql.Open("oracle", "oracle://user:pass@localhost:1521/FREEPDB1")

store := oracle.New(db)
err = store.Migrate(ctx) // or copy schema.sql into your own migrations

ledger := ledgo.New(store) // each Commit runs in its own transaction
```

To commit postings atomically with your own changes, bind the store to your transaction:

```go
sqlTx, err := db.BeginTx(ctx, nil)
defer sqlTx.Rollback()

// ... your business changes on sqlTx ...
err = ledgo.New(oracle.InTx(sqlTx)).Commit(ctx, tx)

err = sqlTx.Commit() // postings and business changes commit together
```

If `Commit` fails, roll back `sqlTx`.

**Call `Migrate` on a store from `New`, never from `InTx`:** Oracle commits DDL implicitly, so it would commit your open transaction.

## Schema

[`schema.sql`](schema.sql) creates three tables: `ledgo_accounts`, `ledgo_transactions` and `ledgo_entries`.

- Amounts are `NUMBER(38, 18)`: up to 20 integer digits and 18 decimal places.
- FX rates are `NUMBER`, with up to 38 decimal places.
- Booleans are `NUMBER(1)`, since `BOOLEAN` needs Oracle 23ai.
- `reference` is nullable, because Oracle stores the empty string as `NULL`.

Amounts are sent and read as text with an explicit decimal separator, so they never go through `float64`, whatever the session's `NLS_NUMERIC_CHARACTERS`.

## Concurrency

Commits lock only accounts with a balance limit, always in ID order, so concurrent commits cannot deadlock on the default `READ COMMITTED` isolation and need no retry loop. Accounts without a limit (clearing, FX, revenue) are insert-only and never locked.

## Limits

- A transaction can touch at most 1000 accounts (Oracle's `IN` list limit).

## Development

```sh
docker compose up -d  # from the repository root; starts Oracle Free 23ai
go test ./...
```
