// Package ledgotest is the conformance suite every ledgo.Store must pass.
//
// Run it from a backend's tests with:
//
//	suite.Run(t, &ledgotest.Suite{Store: store})
package ledgotest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/izzyblues/ledgo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

const (
	workers           = 8
	attemptsPerWorker = 25
)

// Suite runs against Store. Accounts and idempotency keys are unique per run,
// so Store does not need to be empty.
type Suite struct {
	suite.Suite
	Store ledgo.Store

	ledger *ledgo.Ledger
	prefix string
}

// SetupSuite wraps Store in a Ledger. Embedding suites that override it must
// call it.
func (s *Suite) SetupSuite() {
	s.Require().NotNil(s.Store, "ledgotest.Suite needs a Store")
	s.ledger = ledgo.New(s.Store)
	s.prefix = fmt.Sprint(time.Now().UnixNano())
}

// TestBalancedTransfer checks that balances add up across transactions.
func (s *Suite) TestBalancedTransfer() {
	alice, bob := s.open("alice", "USD"), s.open("bob", "USD")
	s.commit(s.transfer("tx1", bob, alice, "10.50"))
	s.commit(s.transfer("tx2", bob, alice, "0.25"))

	s.assertBalance(alice, "-10.75")
	s.assertBalance(bob, "10.75")
}

// TestIdempotency checks that a retry is a no-op and a reused key with
// different content is rejected.
func (s *Suite) TestIdempotency() {
	alice, bob := s.open("alice", "USD"), s.open("bob", "USD")
	tx := s.transfer("tx", bob, alice, "5")
	s.commit(tx)
	s.commit(tx)
	s.assertBalance(bob, "5")

	err := s.ledger.Commit(context.Background(), s.transfer("tx", bob, alice, "6"))
	s.ErrorIs(err, ledgo.ErrIdempotencyConflict)
	s.assertBalance(bob, "5")
}

// TestConcurrentSameKey checks that concurrent commits of one key apply once.
func (s *Suite) TestConcurrentSameKey() {
	alice, bob := s.open("alice", "USD"), s.open("bob", "USD")
	tx := s.transfer("tx", bob, alice, "1")
	s.parallel(func(int) error { return s.ledger.Commit(context.Background(), tx) })
	s.assertBalance(bob, "1")
}

// TestFXConversion checks that a conversion balances per currency through the
// FX accounts.
func (s *Suite) TestFXConversion() {
	alice, fxUSD := s.open("alice", "USD"), s.open("fx-USD", "USD")
	fxEUR, bob := s.open("fx-EUR", "EUR"), s.open("bob", "EUR")
	rate := ledgo.Rate{From: "USD", To: "EUR", Value: decimal.RequireFromString("0.9137"), Source: "test", Time: time.Now()}
	usd := decimal.RequireFromString("100.05")
	eur, err := rate.Convert(usd)
	s.Require().NoError(err)

	// 100.05 * 0.9137 = 91.415685, rounded half-even to 91.42.
	s.commit(ledgo.Transaction{
		IdempotencyKey: s.name("fx"),
		Rate:           &rate,
		Entries: []ledgo.Entry{
			entry(alice, usd.Neg()),
			entry(fxUSD, usd),
			entry(fxEUR, eur.Neg()),
			entry(bob, eur),
		},
	})

	s.assertBalance(alice, "-100.05")
	s.assertBalance(fxUSD, "100.05")
	s.assertBalance(fxEUR, "-91.42")
	s.assertBalance(bob, "91.42")
}

// TestCreateAccount checks that creating an account again is a no-op, and
// that different settings under the same ID are rejected.
func (s *Suite) TestCreateAccount() {
	account := s.open("account", "USD")
	s.Require().NoError(s.ledger.CreateAccount(context.Background(), account))

	account.DebitsMustNotExceedCredits = true
	s.ErrorIs(s.ledger.CreateAccount(context.Background(), account), ledgo.ErrAccountConflict)
}

// TestUnknownAccount checks that commits and balances need a created account.
func (s *Suite) TestUnknownAccount() {
	known := s.open("known", "USD")
	unknown := ledgo.Account{ID: s.name("unknown"), Currency: "USD"}

	err := s.ledger.Commit(context.Background(), s.transfer("tx", known, unknown, "1"))
	s.ErrorIs(err, ledgo.ErrUnknownAccount)
	_, err = s.ledger.Balance(context.Background(), unknown.ID)
	s.ErrorIs(err, ledgo.ErrUnknownAccount)
	s.assertBalance(known, "0")
}

// TestCurrencyMismatch checks that an entry must match its account's currency.
func (s *Suite) TestCurrencyMismatch() {
	usd, eur := s.open("usd", "USD"), s.open("eur", "EUR")
	err := s.ledger.Commit(context.Background(), ledgo.Transaction{
		IdempotencyKey: s.name("tx"),
		Entries: []ledgo.Entry{
			entry(usd, decimal.NewFromInt(1)),
			{Account: eur.ID, Currency: "USD", Amount: decimal.NewFromInt(-1)},
		},
	})
	s.ErrorIs(err, ledgo.ErrCurrencyMismatch)
	s.assertBalance(usd, "0")
}

// TestDebitsMustNotExceedCredits checks that a customer account, which holds
// a credit balance, cannot be overdrawn.
func (s *Suite) TestDebitsMustNotExceedCredits() {
	bank, customer, merchant := s.open("bank", "USD"), s.openCustomer("customer", "USD"), s.open("merchant", "USD")
	s.commit(s.transfer("fund", bank, customer, "10"))

	err := s.ledger.Commit(context.Background(), s.transfer("overdraw", customer, merchant, "10.01"))
	s.ErrorIs(err, ledgo.ErrInsufficientFunds)
	s.assertBalance(customer, "-10")
	s.assertBalance(merchant, "0")

	s.commit(s.transfer("spend", customer, merchant, "10"))
	s.assertBalance(customer, "0")
}

// TestCreditsMustNotExceedDebits checks that a prefund account, which holds a
// debit balance, cannot be overdrawn.
func (s *Suite) TestCreditsMustNotExceedDebits() {
	bank := s.open("bank", "USD")
	prefund := s.create(ledgo.Account{ID: s.name("prefund"), Currency: "USD", CreditsMustNotExceedDebits: true})

	err := s.ledger.Commit(context.Background(), s.transfer("overdraw", bank, prefund, "1"))
	s.ErrorIs(err, ledgo.ErrInsufficientFunds)

	s.commit(s.transfer("top-up", prefund, bank, "5"))
	s.commit(s.transfer("use", bank, prefund, "5"))
	s.assertBalance(prefund, "0")
}

// TestConcurrentOverspend makes 200 concurrent attempts to spend 0.01 from a
// balance of 1.00: exactly 100 must succeed.
func (s *Suite) TestConcurrentOverspend() {
	bank, customer, merchant := s.open("bank", "USD"), s.openCustomer("customer", "USD"), s.open("merchant", "USD")
	s.commit(s.transfer("fund", bank, customer, "1"))

	var spent atomic.Int64
	s.parallel(func(worker int) error {
		for attempt := range attemptsPerWorker {
			err := s.ledger.Commit(context.Background(), s.transfer(fmt.Sprintf("spend-%d-%d", worker, attempt), customer, merchant, "0.01"))
			if errors.Is(err, ledgo.ErrInsufficientFunds) {
				continue
			}
			if err != nil {
				return err
			}
			spent.Add(1)
		}
		return nil
	})

	s.EqualValues(100, spent.Load())
	s.assertBalance(customer, "0")
	s.assertBalance(merchant, "-1")
}

// TestConcurrentTransfers moves money in both directions around rings of
// limited accounts, in two currencies, from many goroutines. Commits lock
// accounts in opposite orders, so a missing lock order shows up as a deadlock
// error. Then it checks that debits equal credits per currency.
func (s *Suite) TestConcurrentTransfers() {
	currencies := []string{"USD", "BTC"}
	rings := map[string][]ledgo.Account{}
	var all []ledgo.Account
	for _, currency := range currencies {
		bank := s.open("bank-"+currency, currency)
		all = append(all, bank)
		for i := range 4 {
			customer := s.openCustomer(fmt.Sprintf("customer-%d-%s", i, currency), currency)
			s.commit(s.transfer("fund-"+customer.ID, bank, customer, "100"))
			rings[currency] = append(rings[currency], customer)
			all = append(all, customer)
		}
	}

	s.parallel(func(worker int) error {
		for attempt := range attemptsPerWorker {
			i := worker*attemptsPerWorker + attempt
			ring := rings[currencies[i%2]]
			from, to := ring[i%4], ring[(i+1)%4]
			if i%8 >= 4 {
				from, to = to, from
			}
			err := s.ledger.Commit(context.Background(), s.transfer(fmt.Sprint(i), from, to, fmt.Sprintf("0.0%d", i%7+1)))
			if err != nil {
				return err
			}
		}
		return nil
	})

	totals := map[string]decimal.Decimal{}
	for _, account := range all {
		totals[account.Currency] = totals[account.Currency].Add(s.balance(account))
	}
	for currency, total := range totals {
		s.Truef(total.IsZero(), "%s: debits and credits differ by %s", currency, total)
	}
}

// name makes account IDs and keys unique to this run and test.
func (s *Suite) name(base string) string {
	return fmt.Sprintf("%s/%s/%s", s.prefix, s.T().Name(), base)
}

func (s *Suite) create(account ledgo.Account) ledgo.Account {
	s.Require().NoError(s.ledger.CreateAccount(context.Background(), account))
	return account
}

// open creates an account with no limit, like a system account.
func (s *Suite) open(name, currency string) ledgo.Account {
	return s.create(ledgo.Account{ID: s.name(name), Currency: currency})
}

// openCustomer creates an account holding a credit balance that must not be
// overdrawn.
func (s *Suite) openCustomer(name, currency string) ledgo.Account {
	return s.create(ledgo.Account{ID: s.name(name), Currency: currency, DebitsMustNotExceedCredits: true})
}

func entry(account ledgo.Account, amount decimal.Decimal) ledgo.Entry {
	return ledgo.Entry{Account: account.ID, Currency: account.Currency, Amount: amount}
}

// transfer debits one account and credits the other with amount.
func (s *Suite) transfer(key string, debit, credit ledgo.Account, amount string) ledgo.Transaction {
	a := decimal.RequireFromString(amount)
	return ledgo.Transaction{
		IdempotencyKey: s.name(key),
		Entries:        []ledgo.Entry{entry(debit, a), entry(credit, a.Neg())},
	}
}

func (s *Suite) commit(tx ledgo.Transaction) {
	s.Require().NoError(s.ledger.Commit(context.Background(), tx), tx.IdempotencyKey)
}

func (s *Suite) balance(account ledgo.Account) decimal.Decimal {
	got, err := s.ledger.Balance(context.Background(), account.ID)
	s.Require().NoError(err)
	return got
}

func (s *Suite) assertBalance(account ledgo.Account, want string) {
	got := s.balance(account)
	s.Truef(got.Equal(decimal.RequireFromString(want)), "Balance(%s) = %s, want %s", account.ID, got, want)
}

// parallel runs fn on several goroutines at once. Errors are collected and
// asserted on the test goroutine, since FailNow must not be called elsewhere.
func (s *Suite) parallel(fn func(worker int) error) {
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() { errs <- fn(worker) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		s.Require().NoError(err)
	}
}
