package ledgo

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// Errors returned for amounts that don't fit the Currencies table.
var (
	ErrUnknownCurrency = errors.New("ledgo: unknown currency")
	ErrTooManyDecimals = errors.New("ledgo: amount has more decimal places than its currency allows")
)

// Currencies maps currency codes to their number of decimal places.
// Add entries to support more currencies.
//
// ponytail: a plain package-level map; make it per-Ledger if two ledgers ever need different tables.
var Currencies = map[string]int32{
	"USD": 2, "EUR": 2, "GBP": 2, "CHF": 2, "CAD": 2, "AUD": 2, "NZD": 2,
	"SGD": 2, "HKD": 2, "CNY": 2, "INR": 2, "MXN": 2, "BRL": 2, "ZAR": 2,
	"SEK": 2, "NOK": 2, "DKK": 2, "PLN": 2, "PHP": 2, "THB": 2, "IDR": 2,
	"JPY": 0, "KRW": 0, "VND": 0, "CLP": 0, "ISK": 0,
	"BHD": 3, "KWD": 3, "OMR": 3, "JOD": 3, "TND": 3,
	"BTC": 8, "USDC": 6, "USDT": 6, "ETH": 18,
}

func checkDecimalPlaces(amount decimal.Decimal, currency string) error {
	places, ok := Currencies[currency]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownCurrency, currency)
	}
	if !amount.Equal(amount.Truncate(places)) {
		return fmt.Errorf("%w: %s %s", ErrTooManyDecimals, amount, currency)
	}
	return nil
}
