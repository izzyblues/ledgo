package ledgo

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestCheckBalance(t *testing.T) {
	customer := Account{ID: "customer", Currency: "USD", DebitsMustNotExceedCredits: true}
	prefund := Account{ID: "prefund", Currency: "USD", CreditsMustNotExceedDebits: true}
	system := Account{ID: "system", Currency: "USD"}
	credit, zero, debit := decimal.RequireFromString("-0.01"), decimal.Zero, decimal.RequireFromString("0.01")

	assert.NoError(t, customer.CheckBalance(credit))
	assert.NoError(t, customer.CheckBalance(zero))
	assert.ErrorIs(t, customer.CheckBalance(debit), ErrInsufficientFunds)

	assert.ErrorIs(t, prefund.CheckBalance(credit), ErrInsufficientFunds)
	assert.NoError(t, prefund.CheckBalance(zero))
	assert.NoError(t, prefund.CheckBalance(debit))

	assert.NoError(t, system.CheckBalance(credit))
	assert.NoError(t, system.CheckBalance(debit))
}

func TestAccountValidate(t *testing.T) {
	assert.NoError(t, Account{ID: "a", Currency: "USD"}.validate())
	assert.ErrorIs(t, Account{Currency: "USD"}.validate(), ErrNoAccount)
	assert.ErrorIs(t, Account{ID: "a", Currency: "XXX"}.validate(), ErrUnknownCurrency)
}
