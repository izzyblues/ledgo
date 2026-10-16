package oracle

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/sijms/go-ora/v2/network"
)

// Oracle error codes.
const (
	uniqueViolation = 1    // ORA-00001: unique constraint violated
	nameInUse       = 955  // ORA-00955: name is already used by an existing object
	alreadyIndexed  = 1408 // ORA-01408: such column list already indexed
)

type dialect struct{}

func (dialect) Placeholder(n int) string { return ":" + strconv.Itoa(n) }

// Number sets the decimal separator explicitly instead of taking it from the
// session's NLS settings.
func (dialect) Number(param string) string {
	return fmt.Sprintf("TO_NUMBER(%s, '%s', %s)", param, numberFormat, decimalPoint)
}

// Text formats a NUMBER as text, because go-ora would scan it as a float64.
func (dialect) Text(expr string) string {
	return fmt.Sprintf("TO_CHAR(%s, 'TM9', %s)", expr, decimalPoint)
}

const decimalPoint = `'NLS_NUMERIC_CHARACTERS=''.,'''`

// numberFormat accepts the 20 integer digits of a NUMBER(38, 18) amount, and
// up to 38 decimal places for rates. Oracle rejects longer format models.
var numberFormat = strings.Repeat("9", 20) + "D" + strings.Repeat("9", 38)

// IgnoreDuplicate leaves the INSERT as it is: Oracle rolls back only the
// statement that hit ORA-00001.
func (dialect) IgnoreDuplicate(insert string) string { return insert }

func (dialect) IsDuplicate(err error) bool { return isOracleError(err, uniqueViolation) }

// TxOptions is nil: READ COMMITTED is the default.
func (dialect) TxOptions() *sql.TxOptions { return nil }

// AlreadyExists stands in for IF NOT EXISTS, which needs Oracle 23ai.
func (dialect) AlreadyExists(err error) bool { return isOracleError(err, nameInUse, alreadyIndexed) }

func isOracleError(err error, codes ...int) bool {
	var oracleErr *network.OracleError
	return errors.As(err, &oracleErr) && slices.Contains(codes, oracleErr.ErrCode)
}
