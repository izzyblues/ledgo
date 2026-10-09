// Package ledgo is a double-entry ledger with multi-currency and FX support.
//
// Amounts are signed: a positive amount debits the account, a negative amount
// credits it. A transaction balances when its amounts sum to zero in every
// currency.
package ledgo

import (
	"context"

	"github.com/shopspring/decimal"
)

// Ledger validates transactions before handing them to a Store.
type Ledger struct {
	store Store
}

// New returns a Ledger backed by store.
func New(store Store) *Ledger {
	return &Ledger{store: store}
}

// Commit validates tx and stores it. See Store.Commit for idempotency.
func (l *Ledger) Commit(ctx context.Context, tx Transaction) error {
	if err := tx.Validate(); err != nil {
		return err
	}
	return l.store.Commit(ctx, tx)
}

// Balance is the sum of all the account's entries in currency.
func (l *Ledger) Balance(ctx context.Context, account, currency string) (decimal.Decimal, error) {
	return l.store.Balance(ctx, account, currency)
}
