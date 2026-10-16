# ledgo

Double-entry ledger library for Go, with multi-currency and FX support, append-only storage and pluggable storage backends.

- Module: `github.com/izzyblues/ledgo` (public on GitHub). Licence: Apache-2.0.
- Status: v0.0.1 (core types, `TransactionBuilder`, validation, conformance suite `ledgotest`, and the PostgreSQL and Oracle backends).
- Releases: tag the core module `vX.Y.Z` first, then each backend (`postgres/vX.Y.Z`, `oracle/vX.Y.Z`). Each backend's `go.mod` requires the matching core version. Local development uses the root `go.work` (add each new backend module to it); consumers ignore it.
- Tests: testify. Postgres and Oracle run in Docker: `docker compose up -d`, then `go test ./...` in the backend's directory.
- One entity per file, each with its own sentinel errors.
- `make` lints and tests every module (`make lint`, `make test`, `make fmt`, `make db-up`). golangci-lint v2, config in `.golangci.yml`. Add each new backend module to `MODULES` in the Makefile, to `go.work`, and to the matrix in `.github/workflows/ci.yml` (with its database as a service).

## Intent

ledgo is **the ledger inside your SQL transaction**: an embeddable library whose postings commit atomically with the application's own business state (`postgres.InTx`). That is its niche. TigerBeetle, Formance and Blnk are separate ledger services, so with them every money movement becomes a two-system consistency problem; with ledgo it is one transaction. ledgo is not a ledger service and should not grow into one.

Consequences for design decisions:

- **Atomicity with the caller's transaction comes first.** Prefer features that work inside `InTx`. A feature that needs its own transaction or process is a sign it belongs in a service, not here.
- **Target: a service's own ledger at moderate volume.** Very high throughput on hot accounts, or one central ledger shared by many services, is TigerBeetle's job.
- **Stay small and general.** Business knowledge (chart of accounts, sign conventions for credit-normal accounts, error mapping) stays in the application. Extract into ledgo only what a real integration proved generic, as with `TransactionBuilder`.
- **Recommended integration pattern:** entities carry their transactions like outbox message records; repositories commit them with `InTx` in the entity's transaction; idempotency keys come from the business event.

## Backends

Implementation order follows **database popularity** (DB-Engines ranking). PostgreSQL came first as the reference implementation. Finish each backend, including passing the conformance suite, before starting the next. Wire-compatible databases reuse an existing module and join its test matrix; give them their own module only if dialect differences force it.

| # | Backend | Module | Amount column | Notes |
|---|---|---|---|---|
| 1 | PostgreSQL | `postgres` | `NUMERIC` | Done. Reference implementation. Also covers Aurora, AlloyDB, Neon and Supabase. |
| 2 | Oracle | `oracle` | `NUMBER(38, 18)` | Done. `sijms/go-ora` (pure Go; avoid cgo `godror`). go-ora reads `NUMBER` as `float64`, so decimals cross the wire as text with an explicit `NLS_NUMERIC_CHARACTERS` (`TO_NUMBER`/`TO_CHAR`). Duplicates are caught as ORA-00001, which rolls back only the statement. `''` is `NULL`, booleans are `NUMBER(1)`, and DDL commits implicitly, so `Migrate` never runs `InTx`. |
| 3 | MySQL | `mysql` | `DECIMAL(38, 18)` | InnoDB only. No `RETURNING`. Watch for gap-lock deadlocks under `FOR UPDATE`. |
| 4 | SQL Server | `sqlserver` | `DECIMAL(38, 18)` | `microsoft/go-mssqldb`. `OUTPUT` instead of `RETURNING`, `WITH (UPDLOCK, ROWLOCK)` instead of `FOR UPDATE`. Upsert with `MERGE` or catch duplicate key errors 2627/2601. |
| 5 | MongoDB | `mongodb` | `Decimal128` | Not SQL: its own implementation behind the same storage interface. Joins the caller's transaction through the session carried in `ctx`. Needs a replica set, even single-node. Limits via a conditional `$inc` filtered on the balance. Write conflicts abort with `TransientTransactionError`; the caller's `WithTransaction` retries. |
| 6 | IBM Db2 | `db2` | `DECIMAL(31, 18)` | `ibmdb/go_ibm_db` (cgo, needs clidriver). Max precision 31, so only 13 integer digits at 18 decimal places: document it. |
| 7 | SQLite | `sqlite` | `TEXT` | No decimal type, so `SUM()` in SQL won't work: compute balances in Go or keep a cached balance. Single writer: use `BEGIN IMMEDIATE` to avoid `SQLITE_BUSY` on lock upgrade. Intended for tests and embedded use. |
| 8 | MariaDB | `mysql` | `DECIMAL(38, 18)` | Same driver as MySQL: add it to the `mysql` test matrix. |
| 9 | Cloud Spanner | `spanner` | `NUMERIC` | `googleapis/go-sql-spanner`. `NUMERIC` holds only 9 decimal places: reject currencies with more. Serializable: retry on `Aborted`. |
| 10 | CockroachDB | `cockroachdb` | `DECIMAL` | Same wire protocol as Postgres, so reuse the `pgx` code. Serializable by default: retry on SQLSTATE `40001`. |
| 11 | TiDB | `mysql` | `DECIMAL(38, 18)` | MySQL protocol: add it to the `mysql` test matrix. Use pessimistic transactions; retry on write conflict (error 9007). |
| 12 | YugabyteDB | `cockroachdb` | `NUMERIC` | Postgres-compatible (YSQL), so reuse the `pgx` code. Retry on SQLSTATE `40001`. |

## Architecture

- **Storage interface at the ledger level, not SQL-shaped** (e.g. `Commit(ctx, tx)` and `Balance(ctx, account)`), so callers never depend on a backend's SQL.
- **One shared SQL implementation** on `database/sql`, plus a small per-database dialect (locking clause, upsert syntax, `RETURNING`/`OUTPUT`, retryable error codes) for every SQL backend. MongoDB is the only non-SQL backend and has its own implementation.
- **One Go module per backend** (`ledgo/postgres`, `ledgo/oracle`, `ledgo/mysql`, `ledgo/sqlserver`, `ledgo/mongodb`, ...), so users only download the driver they use. Wire-compatible databases share a module (see Backends).
- **Shared conformance test suite** that every backend must pass: balanced postings, idempotency, concurrent transfers, FX conversions, and debits = credits per currency under concurrent load.
- Build the retry loop for serialization failures in from day one.

## Money: `shopspring/decimal`

Chosen over `int64` minor units because `int64` can't hold real amounts at 18 decimal places (crypto). Rules:

- **Never use floats**, anywhere: not in the API, not in conversions.
- **Never call `Div` in money paths.** `decimal.DivisionPrecision` is a package-wide setting that anyone can change. For FX, multiply by the rate, then round straight away to the target currency's decimal places.
- **Compare with `Equal`/`Cmp`, never `==`.** The struct holds a `*big.Int` pointer.
- **Validate decimal places on input.** Reject amounts with more decimal places than their currency allows (e.g. `1.005 USD`). Decimal places come from currencies registered with `ledgo.RegisterCurrency` (e.g. crypto, up to 18 places; safe for concurrent use), then from `Rhymond/go-money`'s ISO 4217 table, matched on the exact uppercase code. **go-money is an implementation detail**: never expose it in the API or ask users to import it, and never write to its global table. `TestISOCurrencies` pins a few entries so a go-money upgrade can't silently change validation.
- **`shopspring/decimal` is part of the public API.** Changing it later is a breaking change.

## Accounting rules (non-negotiable)

- **Every transaction balances per currency**, never across currencies.
- **FX goes through trading/conversion accounts.** Each side of a conversion balances in its own currency through the FX account. Reference: Peter Selinger, "Tutorial on multiple currency accounting".
- **The caller supplies the FX rate.** The library never fetches rates. The rate (with its source, timestamp and direction) is stored on the transaction and never recomputed.
- **Rounding is explicit.** Pick one documented mode (`RoundBank` for half-even, or `Round` for half away from zero), and put remainders in a rounding account when needed.
- **Append-only.** No updates or deletes. Corrections are reversal entries.
- **Every posting needs an idempotency key.** Retrying with the same key and same content is a no-op; the same key with different content is an error.

## Out of scope for now

FX revaluation (unrealized gains/losses), reporting/base-currency balances, rate fetching, accounting periods/closing. Pending holds (two-phase transfers) and multi-tenancy are still undecided; holds can be modelled as a separate "held" account.

## Open questions

- Point-in-time balance queries?
- Balances of accounts without a limit are `SUM()`s over all their entries, which slows down as hot accounts grow. Snapshots when it matters.

## Decisions

- **Accounts must be created before use**, hold one currency, and use TigerBeetle-style limits: `DebitsMustNotExceedCredits` / `CreditsMustNotExceedDebits`, failing with `ErrInsufficientFunds`. Balances are always raw debits minus credits; callers flip signs for credit-normal accounts (chosen over a "normal side" field to keep the library small).
- **Only accounts with a limit keep a cached balance**, updated in the commit and locked in sorted ID order, so commits can't deadlock (the conformance suite checks this). Accounts without a limit (system accounts: clearing, FX, revenue) are insert-only, never locked, and their balance is `SUM()` over entries. This is the hot-account answer. Still no retry loop in Postgres; add it with the first backend that needs one.
- **`postgres.InTx(*sql.Tx)`** writes into the caller's transaction, so postings commit atomically with business state.
- **`Transaction.Reference`** links a transaction to what caused it (e.g. a payment ID). It is part of the fingerprint.
- **`TransactionBuilder`** (`NewTransaction(key, ref).Debit(a, x).Credit(b, x).Build()`) is the recommended way to build transactions: entries take their currency from the `Account`, zero amounts are skipped, `Negated()` moves money back, and `Accounts()` lists accounts to create before committing. Builders are immutable values. Extracted because a real integration needed it in every money flow. There is no `Transaction.Reversal(key)` yet: a true reversal of a stored transaction needs a way to load it.
- **Signed amounts**: positive = debit, negative = credit. A transaction balances when each currency sums to zero.
- **Rounding is `RoundBank` (half-even)**, in `Rate.Convert`.
- **Idempotency uses a SHA-256 fingerprint** of the content (excluding the key), stored next to the unique key.
- **Start with the simplest implementation**: no append-only triggers or batching yet.

- **`shopspring/decimal` over the alternatives.** `int64` minor units overflow at 18 decimal places (crypto). `govalues/decimal` is limited to 19 digits, which has the same problem. `cockroachdb/apd` is faster and has no global state, but its API is clunkier. `shopspring/decimal` is the most widely used, and has built-in `sql.Scanner`/`driver.Valuer` and JSON support.
- **Transactional backends only; Cassandra was dropped.** Cassandra cannot join a caller's transaction, which is ledgo's core value (see Intent), and has no cross-partition transactions, so it would be a separate, weaker product behind the same API. Don't add non-transactional backends (Redis, DynamoDB, Elasticsearch) or analytics warehouses (Snowflake, BigQuery, ClickHouse). MongoDB is in because its multi-document transactions let postings commit in the caller's session.
- **Build order follows popularity**, so the README and the build order list backends in the same order. The README is for users and says only which databases are supported and their guarantees; implementation notes stay here.

## Prior art

TigerBeetle (Go client), Formance Ledger (Go), Blnk (Go), pgledger (Postgres). Learn from their designs.
