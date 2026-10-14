package ledgo

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	alice = Account{ID: "alice", Currency: "USD", DebitsMustNotExceedCredits: true}
	bob   = Account{ID: "bob", Currency: "USD"}
)

func TestBuilderDebitsAndCreditsInTheAccountCurrency(t *testing.T) {
	amount := decimal.RequireFromString("10.50")

	tx := NewTransaction("key", "ref").Debit(alice, amount).Credit(bob, amount).Build()

	require.NoError(t, tx.Validate())
	assert.Equal(t, "key", tx.IdempotencyKey)
	assert.Equal(t, "ref", tx.Reference)
	assert.Equal(t, []Entry{
		{Account: "alice", Currency: "USD", Amount: amount},
		{Account: "bob", Currency: "USD", Amount: amount.Neg()},
	}, tx.Entries)
	assert.Nil(t, tx.Rate)
}

func TestBuilderSkipsZeroAmounts(t *testing.T) {
	b := NewTransaction("key", "ref").Debit(alice, decimal.Zero).Credit(bob, decimal.Zero)

	assert.True(t, b.IsEmpty())
	assert.Empty(t, b.Build().Entries)
}

func TestBuilderIsImmutable(t *testing.T) {
	base := NewTransaction("key", "ref").Debit(alice, decimal.NewFromInt(1))

	base.Credit(bob, decimal.NewFromInt(1))
	base.WithRate(Rate{From: "USD", To: "EUR"})

	assert.Len(t, base.Build().Entries, 1)
	assert.Nil(t, base.Build().Rate)
}

func TestBuilderNegatedSwapsDebitsAndCredits(t *testing.T) {
	rate := Rate{From: "USD", To: "EUR", Value: decimal.RequireFromString("0.9"), Source: "test", Time: time.Now()}
	b := NewTransaction("key", "ref").Debit(alice, decimal.NewFromInt(5)).Credit(bob, decimal.NewFromInt(5)).WithRate(rate)

	negated := b.Negated().Build()

	assert.True(t, negated.Entries[0].Amount.Equal(decimal.NewFromInt(-5)))
	assert.True(t, negated.Entries[1].Amount.Equal(decimal.NewFromInt(5)))
	assert.Equal(t, &rate, negated.Rate)
	assert.True(t, b.Build().Entries[0].Amount.Equal(decimal.NewFromInt(5)), "the original is unchanged")
}

func TestBuilderAccountsAreDistinctAndInOrder(t *testing.T) {
	b := NewTransaction("key", "ref").
		Debit(alice, decimal.NewFromInt(1)).
		Credit(bob, decimal.NewFromInt(2)).
		Debit(alice, decimal.NewFromInt(1))

	assert.Equal(t, []Account{alice, bob}, b.Accounts())
}
