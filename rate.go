package ledgo

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// ErrInvalidRate is returned for a rate with unknown or identical currencies,
// a non-positive value, or a missing source or time.
var ErrInvalidRate = errors.New("ledgo: invalid FX rate")

// Rate is the FX rate the caller used: one unit of From buys Value units of To.
// It is stored with the transaction and never recomputed.
type Rate struct {
	From   string
	To     string
	Value  decimal.Decimal
	Source string
	Time   time.Time
}

// Convert multiplies amount by the rate and rounds half-even (RoundBank) to
// the target currency's decimal places. It never divides.
func (r Rate) Convert(amount decimal.Decimal) (decimal.Decimal, error) {
	places, ok := decimalPlaces(r.To)
	if !ok {
		return decimal.Decimal{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, r.To)
	}
	return amount.Mul(r.Value).RoundBank(places), nil
}

func (r *Rate) validate() error {
	if r == nil {
		return nil
	}
	_, fromKnown := decimalPlaces(r.From)
	_, toKnown := decimalPlaces(r.To)
	if !fromKnown || !toKnown || r.From == r.To {
		return fmt.Errorf("%w: %q to %q", ErrInvalidRate, r.From, r.To)
	}
	if !r.Value.IsPositive() || r.Source == "" || r.Time.IsZero() {
		return fmt.Errorf("%w: needs a positive value, a source and a time", ErrInvalidRate)
	}
	return nil
}
