package ledgo

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// Errors returned for invalid entries.
var (
	ErrNoAccount  = errors.New("ledgo: missing account")
	ErrZeroAmount = errors.New("ledgo: zero amount")
)

// Entry is one leg of a transaction. A positive Amount debits the account, a
// negative one credits it.
type Entry struct {
	Account  string
	Currency string
	Amount   decimal.Decimal
}

func (e Entry) validate() error {
	if e.Account == "" {
		return ErrNoAccount
	}
	if e.Amount.IsZero() {
		return fmt.Errorf("%w: account %s", ErrZeroAmount, e.Account)
	}
	return checkDecimalPlaces(e.Amount, e.Currency)
}
