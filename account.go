package ledgo

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// Errors about accounts.
var (
	ErrUnknownAccount    = errors.New("ledgo: unknown account")
	ErrAccountConflict   = errors.New("ledgo: account already exists with different settings")
	ErrCurrencyMismatch  = errors.New("ledgo: entry currency differs from its account's")
	ErrInsufficientFunds = errors.New("ledgo: commit would break an account's balance limit")
)

// Account holds money in a single currency. Its balance is debits minus
// credits.
//
// The two limits work like TigerBeetle's account flags: a commit that would
// make debits exceed credits, or credits exceed debits, fails with
// ErrInsufficientFunds. Accounts with no limit, typically system accounts such
// as clearing, FX or revenue, are never locked by commits.
type Account struct {
	ID                         string
	Currency                   string
	DebitsMustNotExceedCredits bool
	CreditsMustNotExceedDebits bool
}

// HasLimit reports whether either balance limit is set.
func (a Account) HasLimit() bool {
	return a.DebitsMustNotExceedCredits || a.CreditsMustNotExceedDebits
}

// CheckBalance returns ErrInsufficientFunds if balance (debits minus credits)
// breaks the account's limits.
func (a Account) CheckBalance(balance decimal.Decimal) error {
	if a.DebitsMustNotExceedCredits && balance.IsPositive() || a.CreditsMustNotExceedDebits && balance.IsNegative() {
		return fmt.Errorf("%w: %s would be at %s %s", ErrInsufficientFunds, a.ID, balance, a.Currency)
	}
	return nil
}

// CheckEntries checks that every entry's account is in accounts and holds the
// entry's currency. Stores call it before writing entries.
func CheckEntries(entries []Entry, accounts map[string]Account) error {
	for _, e := range entries {
		a, ok := accounts[e.Account]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownAccount, e.Account)
		}
		if a.Currency != e.Currency {
			return fmt.Errorf("%w: %s holds %s, not %s", ErrCurrencyMismatch, a.ID, a.Currency, e.Currency)
		}
	}
	return nil
}

func (a Account) validate() error {
	if a.ID == "" {
		return ErrNoAccount
	}
	if _, ok := decimalPlaces(a.Currency); !ok {
		return fmt.Errorf("%w: %q", ErrUnknownCurrency, a.Currency)
	}
	return nil
}
