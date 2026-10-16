# ledgo/mysql

MySQL store for [ledgo](../README.md), on `database/sql` with the [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) driver. It passes the [`ledgotest`](../ledgotest) conformance suite.

Tested on MySQL 8.4 with InnoDB tables. It needs MySQL 8.0 or later, for the `utf8mb4_0900_bin` collation.

## Installation

```sh
go get github.com/izzyblues/ledgo/mysql@v0.0.1
```

## Usage

```go
db, err := sql.Open("mysql", "user:pass@tcp(localhost:3306)/app")

store := mysql.New(db)
err = store.Migrate(ctx) // or copy schema.sql into your own migrations

ledger := ledgo.New(store) // each Commit runs in its own READ COMMITTED transaction
```

To commit postings atomically with your own changes, bind the store to your transaction, started at READ COMMITTED:

```go
sqlTx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
defer sqlTx.Rollback()

// ... your business changes on sqlTx ...
err = ledgo.New(mysql.InTx(sqlTx)).Commit(ctx, tx)

err = sqlTx.Commit() // postings and business changes commit together
```

If `Commit` fails, roll back `sqlTx`.

**Use READ COMMITTED for `InTx`.** Under MySQL's default REPEATABLE READ, a transaction or account committed by someone else after your transaction's first read is invisible to it, so an idempotent retry can fail instead of being a no-op.

**Keep strict SQL mode on** (`STRICT_TRANS_TABLES`, the default since MySQL 5.7). Without it, MySQL clamps an amount too large for its column to the maximum instead of failing.

**Call `Migrate` on a store from `New`, never from `InTx`:** MySQL commits DDL implicitly, so it would commit your open transaction.

## Schema

[`schema.sql`](schema.sql) creates three InnoDB tables: `ledgo_accounts`, `ledgo_transactions` and `ledgo_entries`.

- Amounts are `DECIMAL(38, 18)`: up to 20 integer digits and 18 decimal places.
- FX rates are `DECIMAL(65, 30)`. MySQL rounds rates with more than 30 decimal places, so keep them within that.
- Account IDs and idempotency keys are at most 768 characters, the most an InnoDB index allows. References are at most 1000.
- Tables use the `utf8mb4_0900_bin` collation, so IDs and keys compare exactly. The default collation ignores case and accents, so `bank` and `BANK` would be the same account.

Amounts are always cast to `DECIMAL` in SQL, so they stay exact with `interpolateParams=true` too, where MySQL would otherwise do the arithmetic in `DOUBLE`.

## Concurrency

Commits lock only accounts with a balance limit, always in ID order, so concurrent commits cannot deadlock and need no retry loop. Accounts without a limit (clearing, FX, revenue) are insert-only and never locked. Balances are updated before entries are inserted, because InnoDB share-locks the accounts that new entries reference.

## Development

```sh
docker compose up -d  # from the repository root; starts MySQL 8.4
go test ./...
```
