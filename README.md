# ledgo

Double-entry ledger library for Go, with multi-currency and FX support, append-only storage and pluggable storage backends.

> **Status:** v0.0.1, the first release. The core API and the PostgreSQL and Oracle backends work and pass the conformance suite. Until v1.0.0 the API may still change between minor versions.

## Installation

ledgo is split into modules, so you only download the driver you use:

```sh
go get github.com/izzyblues/ledgo@v0.0.1          # core: accounts, transactions, validation
go get github.com/izzyblues/ledgo/postgres@v0.0.1 # PostgreSQL store
go get github.com/izzyblues/ledgo/oracle@v0.0.1   # Oracle Database store
```

Create the tables with your store's `Migrate`, for example `postgres.New(db).Migrate(ctx)`, or copy its `schema.sql` into your own migrations. Each store has its own README: [PostgreSQL](postgres/README.md), [Oracle](oracle/README.md).

## Why ledgo

ledgo is a ledger you **embed**, not a ledger service you run. Its postings commit in the **same database transaction** as your own business changes: a payment's status, an outbox event and its ledger entries all commit or roll back together. There is nothing to reconcile between your application and its ledger, because they cannot disagree.

Dedicated ledger services such as [TigerBeetle](https://tigerbeetle.com), [Formance](https://www.formance.com) and [Blnk](https://www.blnkfinance.com) are the better choice when you need very high throughput on contended accounts, or one central ledger shared by many services. They run in their own process and database, so every money movement becomes a two-system consistency problem.

ledgo is a good fit when:

- your service already owns a transactional database (PostgreSQL or Oracle today), and you want its money movements in proper double-entry form;
- you need postings to be atomic with your business state;
- you want SQL access to your ledger for reporting and reconciliation, without running new infrastructure.

## Goals

- **Double-entry:** every transaction balances, per currency.
- **Multi-currency with FX:** conversions go through trading accounts. You supply the rate, and ledgo stores it with the transaction.
- **Exact money:** arbitrary-precision decimals ([shopspring/decimal](https://github.com/shopspring/decimal)), never floats, with correct decimal places per currency, from JPY (0) to crypto (18).
- **Append-only:** entries are never updated or deleted. Corrections are reversal entries.
- **Idempotent:** every transaction carries an idempotency key, so retries are safe.
- **Pluggable storage:** each backend is a separate Go module, so you only download the driver you need.
- **Overdraft protection without hot-account contention:** accounts can forbid going below zero, TigerBeetle-style. Only those accounts are locked by commits. System accounts (clearing, FX, revenue) are insert-only and never locked.

## Example

```go
store := postgres.InTx(sqlTx) // your own transaction
ledger := ledgo.New(store)

customer := ledgo.Account{ID: "customer:42", Currency: "EUR", DebitsMustNotExceedCredits: true}
bank := ledgo.Account{ID: "bank:EUR", Currency: "EUR"}

tx := ledgo.NewTransaction("payment:123:collected", "payment:123").
	Debit(bank, amount).
	Credit(customer, amount).
	Build()

err := ledger.Commit(ctx, tx) // same transaction as your business update
```

## Recommended integration

- **Keep your chart of accounts in your application.** Account names such as `customer:{id}:available` or `clearing:card-acquirer:EUR` are business decisions, not library ones.
- **Let your entities carry their transactions**, like outbox message records: a domain method that moves money records a transaction, and the repository commits it with `postgres.InTx` in the same database transaction as the entity.
- **Derive idempotency keys from the business event** (for example `order:{id}:refunded`), so retries and message redeliveries are no-ops.

## Backends

| Backend | Status | Guarantees |
|---|---|---|
| PostgreSQL (also Aurora, AlloyDB, Neon, Supabase) | Available | Full ACID |
| Oracle | Available | Full ACID |
| MySQL (InnoDB) | Planned | Full ACID |
| SQL Server | Planned | Full ACID |
| MongoDB | Planned | ACID multi-document transactions, replica set required |
| IBM Db2 | Planned | Full ACID, amounts up to 13 integer digits at 18 decimal places |
| SQLite | Planned | Full ACID, single writer |
| MariaDB | Planned | Full ACID |
| Cloud Spanner | Planned | Full ACID, amounts up to 9 decimal places |
| CockroachDB | Planned | Full ACID |
| TiDB | Planned | Full ACID |
| YugabyteDB | Planned | Full ACID |

ledgo supports transactional databases only, because committing inside your own transaction needs one.

## Versioning

ledgo follows [semantic versioning](https://semver.org). Each module is tagged separately: `vX.Y.Z` for the core module and `postgres/vX.Y.Z`, `oracle/vX.Y.Z` and so on for each store. A store release always requires a core release with the same version or newer.

## License

[Apache-2.0](LICENSE)
