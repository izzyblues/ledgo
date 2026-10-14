package ledgo

import (
	"slices"

	"github.com/shopspring/decimal"
)

// TransactionBuilder assembles a Transaction from debits and credits. Each
// entry takes its currency from its account, and zero amounts are skipped, so
// optional legs such as a zero fee need no special casing. Builders are
// values: every method returns a new builder and leaves the receiver as is.
type TransactionBuilder struct {
	idempotencyKey string
	reference      string
	legs           []leg
	rate           *Rate
}

type leg struct {
	account Account
	amount  decimal.Decimal
}

// NewTransaction starts a transaction with its idempotency key and the
// reference to what caused it.
func NewTransaction(idempotencyKey, reference string) TransactionBuilder {
	return TransactionBuilder{idempotencyKey: idempotencyKey, reference: reference}
}

// Debit adds a debit of amount to account.
func (b TransactionBuilder) Debit(account Account, amount decimal.Decimal) TransactionBuilder {
	return b.with(leg{account: account, amount: amount})
}

// Credit adds a credit of amount to account.
func (b TransactionBuilder) Credit(account Account, amount decimal.Decimal) TransactionBuilder {
	return b.with(leg{account: account, amount: amount.Neg()})
}

// WithRate records the FX rate used by a transaction that converts currencies.
func (b TransactionBuilder) WithRate(rate Rate) TransactionBuilder {
	b.rate = &rate
	return b
}

// Negated turns every debit into a credit and every credit into a debit, for
// example to move money back the way it came.
func (b TransactionBuilder) Negated() TransactionBuilder {
	negated := b
	negated.legs = make([]leg, len(b.legs))
	for i, l := range b.legs {
		negated.legs[i] = leg{account: l.account, amount: l.amount.Neg()}
	}
	return negated
}

// IsEmpty reports whether the transaction has no entries, for instance
// because all its amounts were zero.
func (b TransactionBuilder) IsEmpty() bool {
	return len(b.legs) == 0
}

// Accounts returns the distinct accounts the transaction touches, in the order
// they were added, so callers can create them before committing.
func (b TransactionBuilder) Accounts() []Account {
	var accounts []Account
	for _, l := range b.legs {
		if !slices.Contains(accounts, l.account) {
			accounts = append(accounts, l.account)
		}
	}
	return accounts
}

// Build returns the transaction. It does not validate it: Ledger.Commit does.
func (b TransactionBuilder) Build() Transaction {
	entries := make([]Entry, len(b.legs))
	for i, l := range b.legs {
		entries[i] = Entry{Account: l.account.ID, Currency: l.account.Currency, Amount: l.amount}
	}
	return Transaction{IdempotencyKey: b.idempotencyKey, Reference: b.reference, Entries: entries, Rate: b.rate}
}

func (b TransactionBuilder) with(l leg) TransactionBuilder {
	if l.amount.IsZero() {
		return b
	}
	b.legs = append(slices.Clone(b.legs), l)
	return b
}
