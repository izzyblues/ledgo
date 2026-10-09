package ledgo

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Errors returned by Transaction.Validate.
var (
	ErrNoIdempotencyKey = errors.New("ledgo: missing idempotency key")
	ErrTooFewEntries    = errors.New("ledgo: a transaction needs at least two entries")
	ErrUnbalanced       = errors.New("ledgo: transaction does not balance")
)

// Transaction is a set of entries committed atomically.
type Transaction struct {
	IdempotencyKey string
	Entries        []Entry
	// Rate is the FX rate used, for transactions that convert currencies.
	Rate *Rate
}

// Validate checks that tx is well formed and balances in every currency.
func (tx Transaction) Validate() error {
	if tx.IdempotencyKey == "" {
		return ErrNoIdempotencyKey
	}
	if len(tx.Entries) < 2 {
		return ErrTooFewEntries
	}
	if err := validateEntries(tx.Entries); err != nil {
		return err
	}
	if err := checkBalanced(tx.Entries); err != nil {
		return err
	}
	return tx.Rate.validate()
}

func validateEntries(entries []Entry) error {
	for _, e := range entries {
		if err := e.validate(); err != nil {
			return err
		}
	}
	return nil
}

func checkBalanced(entries []Entry) error {
	sums := map[string]decimal.Decimal{}
	for _, e := range entries {
		sums[e.Currency] = sums[e.Currency].Add(e.Amount)
	}
	for currency, sum := range sums {
		if !sum.IsZero() {
			return fmt.Errorf("%w: %s is off by %s", ErrUnbalanced, currency, sum)
		}
	}
	return nil
}

// Fingerprint is a hash of the transaction's content, excluding its
// idempotency key. Stores use it to tell a retry from a conflicting reuse.
func (tx Transaction) Fingerprint() []byte {
	var content []byte
	for _, e := range tx.Entries {
		content = fmt.Appendf(content, "%q %q %s\n", e.Account, e.Currency, e.Amount)
	}
	if r := tx.Rate; r != nil {
		content = fmt.Appendf(content, "rate %q %q %s %q %s\n", r.From, r.To, r.Value, r.Source, r.Time.UTC().Format(time.RFC3339Nano))
	}
	sum := sha256.Sum256(content)
	return sum[:]
}
