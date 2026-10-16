package ledgo

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Rhymond/go-money"
	"github.com/shopspring/decimal"
)

// Errors returned for amounts that don't fit their currency.
var (
	ErrUnknownCurrency = errors.New("ledgo: unknown currency")
	ErrTooManyDecimals = errors.New("ledgo: amount has more decimal places than its currency allows")
)

var registered = struct {
	sync.RWMutex
	places map[string]int32
}{places: map[string]int32{}}

// MaxDecimalPlaces is the most decimal places a currency can have: the stores'
// amount columns keep 18, and some databases round extra places silently.
const MaxDecimalPlaces = 18

// RegisterCurrency adds a currency that the ISO 4217 table doesn't know, such
// as crypto, or overrides one. Codes are case-sensitive. It is safe for
// concurrent use. It panics if decimalPlaces is not between 0 and
// MaxDecimalPlaces.
func RegisterCurrency(code string, decimalPlaces int32) {
	if decimalPlaces < 0 || decimalPlaces > MaxDecimalPlaces {
		panic(fmt.Sprintf("ledgo: %s cannot have %d decimal places, only 0 to %d", code, decimalPlaces, MaxDecimalPlaces))
	}
	registered.Lock()
	defer registered.Unlock()
	registered.places[code] = decimalPlaces
}

// decimalPlaces looks currency up in the registered currencies, then in
// go-money's ISO 4217 table, matching the exact uppercase code.
func decimalPlaces(currency string) (int32, bool) {
	registered.RLock()
	places, ok := registered.places[currency]
	registered.RUnlock()
	if ok {
		return places, true
	}
	c := money.GetCurrency(currency)
	if c == nil || c.Code != currency {
		return 0, false
	}
	return int32(c.Fraction), true //nolint:gosec // ISO 4217 fractions are at most 4
}

func checkDecimalPlaces(amount decimal.Decimal, currency string) error {
	places, ok := decimalPlaces(currency)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownCurrency, currency)
	}
	if !amount.Equal(amount.Truncate(places)) {
		return fmt.Errorf("%w: %s %s", ErrTooManyDecimals, amount, currency)
	}
	return nil
}
