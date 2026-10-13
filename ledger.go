// Package ledgo is a double-entry ledger with multi-currency and FX support.
//
// Amounts are signed: a positive amount debits the account, a negative amount
// credits it. A transaction balances when its amounts sum to zero in every
// currency. Every account holds a single currency and must be created before
// it is used.
package ledgo

import (
	"context"

	"github.com/shopspring/decimal"
)

// Ledger validates accounts and transactions before handing them to a Store.
type Ledger struct {
	store Store
}

// New returns a Ledger backed by store.
func New(store Store) *Ledger {
	return &Ledger{store: store}
}

// CreateAccount validates account and registers it. See Store.CreateAccount.
func (l *Ledger) CreateAccount(ctx context.Context, account Account) error {
	if err := account.validate(); err != nil {
		return err
	}
	return l.store.CreateAccount(ctx, account)
}

// Commit validates tx and stores it. See Store.Commit.
func (l *Ledger) Commit(ctx context.Context, tx Transaction) error {
	if err := tx.Validate(); err != nil {
		return err
	}
	return l.store.Commit(ctx, tx)
}

// Balance is the account's debits minus credits.
func (l *Ledger) Balance(ctx context.Context, account string) (decimal.Decimal, error) {
	return l.store.Balance(ctx, account)
}
