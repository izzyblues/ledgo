package sqlstore

import "database/sql"

// Dialect is what differs between the SQL databases. Queries are written with
// "?" placeholders, which Store rewrites with Placeholder.
type Dialect interface {
	// Placeholder is the bind parameter for the nth argument, from 1: "$1",
	// ":1" or "?".
	Placeholder(n int) string
	// Number converts the decimal bound to param, which shopspring/decimal
	// sends as text with a '.' separator, to an exact number.
	Number(param string) string
	// Text formats a numeric expression so it scans into a decimal.Decimal
	// exactly, never through float64.
	Text(expr string) string
	// IgnoreDuplicate rewrites a single-row INSERT so that a duplicate key
	// inserts nothing, for databases where the error would abort the whole
	// transaction.
	IgnoreDuplicate(insert string) string
	// IsDuplicate reports whether err is a unique key violation. The database
	// must have rolled back only the failed statement, so the transaction
	// stays usable.
	IsDuplicate(err error) bool
	// TxOptions starts the transactions that Store.Commit opens itself. They
	// must run at READ COMMITTED: nil if that is the database's default.
	TxOptions() *sql.TxOptions
	// AlreadyExists reports whether a Migrate statement failed because its
	// table or index already exists.
	AlreadyExists(err error) bool
}
