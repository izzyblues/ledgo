# ledgo

Double-entry ledger library for Go, with multi-currency and FX support, append-only storage and pluggable storage backends.

> **Status:** early development. There is no usable API yet.

## Goals

- **Double-entry:** every transaction balances, per currency.
- **Multi-currency with FX:** conversions go through trading accounts. You supply the rate, and ledgo stores it with the transaction.
- **Exact money:** arbitrary-precision decimals ([shopspring/decimal](https://github.com/shopspring/decimal)), never floats, with correct decimal places per currency, from JPY (0) to crypto (18).
- **Append-only:** entries are never updated or deleted. Corrections are reversal entries.
- **Idempotent:** every transaction carries an idempotency key, so retries are safe.
- **Pluggable storage:** each backend is a separate Go module, so you only download the driver you need.

## Planned backends

| Backend | Guarantees |
|---|---|
| PostgreSQL | Full ACID |
| MySQL (InnoDB) | Full ACID |
| SQLite | Full ACID, single writer |
| Cassandra | Eventually consistent balances, single-account overdraft checks only |
| CockroachDB | Full ACID |

## License

[Apache-2.0](LICENSE)
