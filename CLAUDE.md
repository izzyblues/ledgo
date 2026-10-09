# ledgo

Double-entry ledger library for Go, with multi-currency and FX support, append-only storage and pluggable storage backends.

- Module: `github.com/izzyblues/ledgo` (public on GitHub). Licence: Apache-2.0.
- Status: core types, validation and conformance suite (`ledgotest`, a testify suite) done. PostgreSQL backend in progress (`postgres/`, uses a `replace` to `../` until ledgo is tagged).
- Tests: testify. Postgres runs in Docker: `docker compose up -d`, then `go test ./...` in `postgres/`.
- One entity per file, each with its own sentinel errors.
- `make` lints and tests every module (`make lint`, `make test`, `make fmt`, `make db-up`). golangci-lint v2, config in `.golangci.yml`. Add each new backend module to `MODULES` in the Makefile.

## Backends

Implementation order: **PostgreSQL → MySQL → SQLite → CockroachDB → Cassandra**. Finish each backend, including passing the conformance suite, before starting the next.

| # | Backend | Amount column | Notes |
|---|---|---|---|
| 1 | PostgreSQL | `NUMERIC` | Reference implementation. |
| 2 | MySQL | `DECIMAL(38, 18)` | InnoDB only. No `RETURNING`. Watch for gap-lock deadlocks under `FOR UPDATE`. |
| 3 | SQLite | `TEXT` | No decimal type, so `SUM()` in SQL won't work: compute balances in Go or keep a cached balance. Single writer: use `BEGIN IMMEDIATE` to avoid `SQLITE_BUSY` on lock upgrade. Intended for tests and embedded use. |
| 4 | CockroachDB | `DECIMAL` | Same wire protocol as Postgres, so reuse the `pgx` code. Serializable by default: retry on SQLSTATE `40001`. |
| 5 | Cassandra | `decimal` | `gocql` uses `inf.Dec`, so convert at the boundary. No cross-partition ACID: LWT works within one partition only, and logged batches are atomic but not isolated. Needs its own design: write the transaction once, keyed by its idempotency key, and derive balances asynchronously. Balances are eventually consistent, and overdraft checks are single-account only. These weaker guarantees must be documented. |

## Architecture

- **Storage interface at the ledger level, not SQL-shaped** (e.g. `Commit(ctx, tx)` and `Balance(ctx, account, currency)`), so Cassandra can implement it.
- **One shared SQL implementation** on `database/sql`, plus a small per-database dialect (locking clause, upsert syntax, retryable error codes) for Postgres, CockroachDB, MySQL and SQLite.
- **One Go module per backend** (`ledgo/postgres`, `ledgo/mysql`, `ledgo/sqlite`, `ledgo/cassandra`), so users only download the driver they use.
- **Shared conformance test suite** that every backend must pass: balanced postings, idempotency, concurrent transfers, FX conversions, and debits = credits per currency under concurrent load.
- Build the retry loop for serialization failures in from day one.

## Money: `shopspring/decimal`

Chosen over `int64` minor units because `int64` can't hold real amounts at 18 decimal places (crypto). Rules:

- **Never use floats**, anywhere: not in the API, not in conversions.
- **Never call `Div` in money paths.** `decimal.DivisionPrecision` is a package-wide setting that anyone can change. For FX, multiply by the rate, then round straight away to the target currency's decimal places.
- **Compare with `Equal`/`Cmp`, never `==`.** The struct holds a `*big.Int` pointer.
- **Validate decimal places on input.** Keep a per-currency exponent table (JPY 0, USD 2, BHD 3, crypto up to 18) and reject amounts with more decimal places than their currency allows (e.g. `1.005 USD`).
- **`shopspring/decimal` is part of the public API.** Changing it later is a breaking change.

## Accounting rules (non-negotiable)

- **Every transaction balances per currency**, never across currencies.
- **FX goes through trading/conversion accounts.** Each side of a conversion balances in its own currency through the FX account. Reference: Peter Selinger, "Tutorial on multiple currency accounting".
- **The caller supplies the FX rate.** The library never fetches rates. The rate (with its source, timestamp and direction) is stored on the transaction and never recomputed.
- **Rounding is explicit.** Pick one documented mode (`RoundBank` for half-even, or `Round` for half away from zero), and put remainders in a rounding account when needed.
- **Append-only.** No updates or deletes. Corrections are reversal entries.
- **Every posting needs an idempotency key.** Retrying with the same key and same content is a no-op; the same key with different content is an error.

## Out of scope for now

FX revaluation (unrealized gains/losses), reporting/base-currency balances, rate fetching, accounting periods/closing. Pending holds (two-phase transfers), overdraft rules, account normal balances and multi-tenancy are still undecided.

## Open questions

- Point-in-time balance queries?
- Hot accounts (fee/omnibus accounts touched by every posting): locking strategy, once cached balances or overdraft checks exist.

## Decisions

- **Balances are `SUM()` over entries**, no cached balance. Commits only insert rows, so they take no locks beyond their own idempotency key and cannot deadlock on READ COMMITTED. No retry loop in Postgres for now; add it when a backend needs one (CockroachDB) or when cached balances bring row locks.
- **Signed amounts**: positive = debit, negative = credit. A transaction balances when each currency sums to zero.
- **Rounding is `RoundBank` (half-even)**, in `Rate.Convert`.
- **Idempotency uses a SHA-256 fingerprint** of the content (excluding the key), stored next to the unique key.
- **Start with the simplest implementation**: no append-only triggers or batching yet.

- **`shopspring/decimal` over the alternatives.** `int64` minor units overflow at 18 decimal places (crypto). `govalues/decimal` is limited to 19 digits, which has the same problem. `cockroachdb/apd` is faster and has no global state, but its API is clunkier. `shopspring/decimal` is the most widely used, and has built-in `sql.Scanner`/`driver.Valuer` and JSON support.
- **CockroachDB is built before Cassandra.** CockroachDB uses the Postgres wire protocol, so it mostly reuses the Postgres code and only needs a serialization-retry loop. Cassandra has no cross-partition transactions, so it needs a separate design with weaker guarantees. Build the cheap backend first, and the hard one last.
- **README lists backends by popularity, not build order.** The README is for users, who care about which databases are supported. The build order (above) is for development. Keep them separate.

## Prior art

TigerBeetle (Go client), Formance Ledger (Go), Blnk (Go), pgledger (Postgres). Learn from their designs.
