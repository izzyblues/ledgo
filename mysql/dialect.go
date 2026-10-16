package mysql

import (
	"database/sql"
	"errors"

	"github.com/go-sql-driver/mysql"
)

// duplicateEntry is MySQL error 1062, ER_DUP_ENTRY.
const duplicateEntry = 1062

type dialect struct{}

func (dialect) Placeholder(int) string { return "?" }

// Number casts the decimal explicitly. With interpolateParams=true the driver
// inlines it as a string literal, and MySQL evaluates balance + '1.5' in
// DOUBLE, losing digits.
func (dialect) Number(param string) string { return "CAST(" + param + " AS DECIMAL(65, 30))" }

// Text passes decimals through: the driver scans DECIMAL as text.
func (dialect) Text(expr string) string { return expr }

// IgnoreDuplicate leaves the INSERT as it is: InnoDB rolls back only the
// statement that hit a duplicate key. INSERT IGNORE would also hide other
// errors.
func (dialect) IgnoreDuplicate(insert string) string { return insert }

func (dialect) IsDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}

// TxOptions asks for READ COMMITTED, since MySQL defaults to REPEATABLE READ.
func (dialect) TxOptions() *sql.TxOptions { return &sql.TxOptions{Isolation: sql.LevelReadCommitted} }

// AlreadyExists is never needed: the schema uses IF NOT EXISTS.
func (dialect) AlreadyExists(error) bool { return false }
