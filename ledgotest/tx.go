package ledgotest

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/izzyblues/ledgo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

// TxSuite runs against a Store bound to a caller's transaction, such as
// postgres.InTx. Run it next to Suite with:
//
//	suite.Run(t, &ledgotest.TxSuite{Store: store, Begin: begin})
type TxSuite struct {
	suite.Suite
	// Store reads balances outside the caller's transactions.
	Store ledgo.Store
	// Begin starts a caller transaction. It returns a Store that writes into
	// it, and end, which commits the transaction or rolls it back.
	Begin func() (store ledgo.Store, end func(commit bool) error, err error)

	prefix string
}

// EndTx ends a database/sql transaction, for TxSuite.Begin.
func EndTx(tx *sql.Tx) func(commit bool) error {
	return func(commit bool) error {
		if commit {
			return tx.Commit()
		}
		return tx.Rollback()
	}
}

// SetupSuite registers ETH, which has 18 decimal places. Embedding suites
// that override it must call it.
func (s *TxSuite) SetupSuite() {
	s.Require().NotNil(s.Store, "ledgotest.TxSuite needs a Store")
	s.Require().NotNil(s.Begin, "ledgotest.TxSuite needs Begin")
	ledgo.RegisterCurrency("ETH", 18)
	s.prefix = fmt.Sprint(time.Now().UnixNano())
}

// TestCommitsWithCallerTransaction checks that postings commit with the
// caller's transaction.
func (s *TxSuite) TestCommitsWithCallerTransaction() {
	bank, customer := s.open("bank", "USD"), s.openCustomer("customer", "USD")
	s.commitInTx(s.transfer(bank, customer, "1"), true)
	s.assertBalance(customer, "-1")
}

// TestRollsBackWithCallerTransaction checks that postings roll back with the
// caller's transaction.
func (s *TxSuite) TestRollsBackWithCallerTransaction() {
	bank, customer := s.open("bank", "USD"), s.openCustomer("customer", "USD")
	s.commitInTx(s.transfer(bank, customer, "1"), false)
	s.assertBalance(customer, "0")
}

// TestExactAmounts checks that an amount with 20 integer digits and 18
// decimal places stays exact, through a cached balance and a summed one.
// Begin can make the caller's session hostile, for example with a ',' decimal
// separator.
func (s *TxSuite) TestExactAmounts() {
	from := ledgo.Account{ID: s.name("from"), Currency: "ETH"}
	to := ledgo.Account{ID: s.name("to"), Currency: "ETH", CreditsMustNotExceedDebits: true}
	amount := decimal.RequireFromString("12345678901234567890.123456789012345678")
	builder := ledgo.NewTransaction(s.name("tx"), "").Debit(to, amount).Credit(from, amount)

	store, end, err := s.Begin()
	s.Require().NoError(err)
	ledger := ledgo.New(store)
	for _, account := range builder.Accounts() {
		s.Require().NoError(ledger.CreateAccount(context.Background(), account))
	}
	s.Require().NoError(ledger.Commit(context.Background(), builder.Build()))
	s.Require().NoError(end(true))

	s.assertBalance(from, amount.Neg().String())
	s.assertBalance(to, amount.String())
}

// name makes account IDs and keys unique to this run and test.
func (s *TxSuite) name(base string) string {
	return fmt.Sprintf("%s/%s/%s", s.prefix, s.T().Name(), base)
}

// open creates an account with no limit, outside any caller transaction.
func (s *TxSuite) open(name, currency string) ledgo.Account {
	return s.create(ledgo.Account{ID: s.name(name), Currency: currency})
}

// openCustomer creates an account holding a credit balance that must not be
// overdrawn, so commits update its cached balance.
func (s *TxSuite) openCustomer(name, currency string) ledgo.Account {
	return s.create(ledgo.Account{ID: s.name(name), Currency: currency, DebitsMustNotExceedCredits: true})
}

func (s *TxSuite) create(account ledgo.Account) ledgo.Account {
	s.Require().NoError(ledgo.New(s.Store).CreateAccount(context.Background(), account))
	return account
}

// transfer debits one account and credits the other with amount.
func (s *TxSuite) transfer(debit, credit ledgo.Account, amount string) ledgo.Transaction {
	return ledgo.NewTransaction(s.name("tx"), "").
		Debit(debit, decimal.RequireFromString(amount)).
		Credit(credit, decimal.RequireFromString(amount)).
		Build()
}

// commitInTx commits tx in a caller transaction, then commits that
// transaction or rolls it back.
func (s *TxSuite) commitInTx(tx ledgo.Transaction, commit bool) {
	store, end, err := s.Begin()
	s.Require().NoError(err)
	s.Require().NoError(ledgo.New(store).Commit(context.Background(), tx))
	s.Require().NoError(end(commit))
}

func (s *TxSuite) assertBalance(account ledgo.Account, want string) {
	got, err := ledgo.New(s.Store).Balance(context.Background(), account.ID)
	s.Require().NoError(err)
	s.Truef(got.Equal(decimal.RequireFromString(want)), "Balance(%s) = %s, want %s", account.ID, got, want)
}
