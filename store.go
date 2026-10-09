package ledgo

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
)

// ErrIdempotencyConflict is returned when an idempotency key is committed
// again with different content.
var ErrIdempotencyConflict = errors.New("ledgo: idempotency key reused with different content")

// Store persists transactions. Implementations live in their own modules and
// must pass the ledgotest conformance suite.
type Store interface {
	// Commit stores tx atomically. Committing the same idempotency key with
	// the same content again is a no-op; with different content it returns
	// ErrIdempotencyConflict.
	Commit(ctx context.Context, tx Transaction) error
	// Balance is the sum of all the account's entries in currency.
	Balance(ctx context.Context, account, currency string) (decimal.Decimal, error)
}
