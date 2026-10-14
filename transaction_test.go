package ledgo

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	usd := func(account, amount string) Entry {
		return Entry{Account: account, Currency: "USD", Amount: decimal.RequireFromString(amount)}
	}
	jpy := func(account, amount string) Entry {
		return Entry{Account: account, Currency: "JPY", Amount: decimal.RequireFromString(amount)}
	}
	rate := &Rate{From: "USD", To: "JPY", Value: decimal.RequireFromString("150"), Source: "test", Time: time.Now()}

	tests := []struct {
		name string
		tx   Transaction
		want error
	}{
		{"balanced", Transaction{IdempotencyKey: "k", Entries: []Entry{usd("a", "-10.00"), usd("b", "10")}}, nil},
		{"no key", Transaction{Entries: []Entry{usd("a", "-1"), usd("b", "1")}}, ErrNoIdempotencyKey},
		{"one entry", Transaction{IdempotencyKey: "k", Entries: []Entry{usd("a", "1")}}, ErrTooFewEntries},
		{"no account", Transaction{IdempotencyKey: "k", Entries: []Entry{usd("", "-1"), usd("b", "1")}}, ErrNoAccount},
		{"zero amount", Transaction{IdempotencyKey: "k", Entries: []Entry{usd("a", "0"), usd("b", "0")}}, ErrZeroAmount},
		{"unknown currency", Transaction{IdempotencyKey: "k", Entries: []Entry{{Account: "a", Currency: "XXX", Amount: decimal.NewFromInt(1)}, usd("b", "-1")}}, ErrUnknownCurrency},
		{"too many decimals", Transaction{IdempotencyKey: "k", Entries: []Entry{usd("a", "-1.005"), usd("b", "1.005")}}, ErrTooManyDecimals},
		{"JPY has no decimals", Transaction{IdempotencyKey: "k", Entries: []Entry{jpy("a", "-1.5"), jpy("b", "1.5")}}, ErrTooManyDecimals},
		{"unbalanced", Transaction{IdempotencyKey: "k", Entries: []Entry{usd("a", "-10"), usd("b", "9.99")}}, ErrUnbalanced},
		{"balances across but not per currency", Transaction{IdempotencyKey: "k", Entries: []Entry{usd("a", "-10"), jpy("b", "10")}}, ErrUnbalanced},
		{"fx", Transaction{IdempotencyKey: "k", Rate: rate, Entries: []Entry{usd("a", "-1"), usd("fx", "1"), jpy("fx", "-150"), jpy("b", "150")}}, nil},
		{"fx rate without source", Transaction{IdempotencyKey: "k", Rate: &Rate{From: "USD", To: "JPY", Value: decimal.NewFromInt(150), Time: time.Now()}, Entries: []Entry{usd("a", "-1"), usd("b", "1")}}, ErrInvalidRate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tx.Validate()
			if tt.want == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tt.want)
			}
		})
	}
}

func TestConvertRoundsHalfEven(t *testing.T) {
	rate := Rate{From: "USD", To: "EUR", Value: decimal.RequireFromString("0.5")}
	for amount, want := range map[string]string{"0.05": "0.02", "0.07": "0.04", "0.03": "0.02"} {
		got, err := rate.Convert(decimal.RequireFromString(amount))
		require.NoError(t, err)
		assert.Truef(t, got.Equal(decimal.RequireFromString(want)), "Convert(%s) = %s, want %s", amount, got, want)
	}
}

func TestFingerprintIgnoresTrailingZerosAndKey(t *testing.T) {
	a := Transaction{IdempotencyKey: "a", Entries: []Entry{{Account: "x", Currency: "USD", Amount: decimal.RequireFromString("10.00")}}}
	b := Transaction{IdempotencyKey: "b", Entries: []Entry{{Account: "x", Currency: "USD", Amount: decimal.RequireFromString("10")}}}
	assert.Equal(t, a.Fingerprint(), b.Fingerprint())
}
