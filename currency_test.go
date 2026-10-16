package ledgo

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// TestISOCurrencies pins a few entries of go-money's table, so an upgrade
// that changes them fails here first.
func TestISOCurrencies(t *testing.T) {
	for code, want := range map[string]int32{"JPY": 0, "USD": 2, "EUR": 2, "IDR": 2, "BHD": 3} {
		got, ok := decimalPlaces(code)
		assert.True(t, ok, code)
		assert.Equal(t, want, got, code)
	}
	for _, code := range []string{"XXX", "usd", "BTC"} {
		_, ok := decimalPlaces(code)
		assert.False(t, ok, code)
	}
}

func TestRegisterCurrency(t *testing.T) {
	RegisterCurrency("ETH", 18)
	RegisterCurrency("POINTS", 0)

	got, ok := decimalPlaces("ETH")
	assert.True(t, ok)
	assert.Equal(t, int32(18), got)
	assert.NoError(t, checkDecimalPlaces(decimal.RequireFromString("0.000000000000000001"), "ETH"))
	assert.ErrorIs(t, checkDecimalPlaces(decimal.RequireFromString("1.5"), "POINTS"), ErrTooManyDecimals)
}

// TestRegisterCurrencyRejectsPlacesStoresCannotKeep checks the 0 to 18 range,
// since Oracle and MySQL would round extra places silently.
func TestRegisterCurrencyRejectsPlacesStoresCannotKeep(t *testing.T) {
	assert.Panics(t, func() { RegisterCurrency("TOKEN", MaxDecimalPlaces+1) })
	assert.Panics(t, func() { RegisterCurrency("TOKEN", -1) })
	_, ok := decimalPlaces("TOKEN")
	assert.False(t, ok)
}
