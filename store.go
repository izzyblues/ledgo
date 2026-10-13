package ledgo

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
)

// ErrIdempotencyConflict is returned when an idempotency key is committed
// again with different content.
var ErrIdempotencyConflict = errors.New("ledgo: idempotency key reused with different content")

// Store persists accounts and transactions. Implementations live in their own
// modules and must pass the ledgotest conformance suite.
type Store interface {
	// CreateAccount registers an account. Creating the same account again is
	// a no-op; with different settings it returns ErrAccountConflict.
	CreateAccount(ctx context.Context, account Account) error
	// Commit stores tx atomically. Committing the same idempotency key with
	// the same content again is a no-op; with different content it returns
	// ErrIdempotencyConflict. It returns ErrUnknownAccount,
	// ErrCurrencyMismatch or ErrInsufficientFunds without storing anything.
	Commit(ctx context.Context, tx Transaction) error
	// Balance is the account's debits minus credits.
	Balance(ctx context.Context, account string) (decimal.Decimal, error)
}
