package postgres

import (
	"database/sql"
	"strconv"
)

type dialect struct{}

func (dialect) Placeholder(n int) string { return "$" + strconv.Itoa(n) }

// Number and Text pass decimals through: pgx sends and scans NUMERIC as text.
func (dialect) Number(param string) string { return param }

func (dialect) Text(expr string) string { return expr }

// IgnoreDuplicate uses ON CONFLICT, because any error, a unique violation
// included, aborts a PostgreSQL transaction.
func (dialect) IgnoreDuplicate(insert string) string { return insert + " ON CONFLICT DO NOTHING" }

func (dialect) IsDuplicate(error) bool { return false }

// TxOptions is nil: READ COMMITTED is the default.
func (dialect) TxOptions() *sql.TxOptions { return nil }

// AlreadyExists is never needed: the schema uses IF NOT EXISTS.
func (dialect) AlreadyExists(error) bool { return false }
