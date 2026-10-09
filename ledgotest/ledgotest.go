// Package ledgotest is the conformance suite every ledgo.Store must pass.
//
// Run it from a backend's tests with:
//
//	suite.Run(t, &ledgotest.Suite{Store: store})
package ledgotest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/izzyblues/ledgo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

const (
	workers            = 8
	transfersPerWorker = 25
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
	s.commit(s.transfer("tx1", "USD", "10.50"))
	s.commit(s.transfer("tx2", "USD", "0.25"))

	s.assertBalance("alice", "USD", "-10.75")
	s.assertBalance("bob", "USD", "10.75")
	s.assertBalance("bob", "EUR", "0")
}

// TestIdempotency checks that a retry is a no-op and a reused key with
// different content is rejected.
func (s *Suite) TestIdempotency() {
	tx := s.transfer("tx", "USD", "5")
	s.commit(tx)
	s.commit(tx)
	s.assertBalance("bob", "USD", "5")

	err := s.ledger.Commit(context.Background(), s.transfer("tx", "USD", "6"))
	s.ErrorIs(err, ledgo.ErrIdempotencyConflict)
	s.assertBalance("bob", "USD", "5")
}

// TestConcurrentSameKey checks that concurrent commits of one key apply once.
func (s *Suite) TestConcurrentSameKey() {
	tx := s.transfer("tx", "USD", "1")
	s.parallel(func() error { return s.ledger.Commit(context.Background(), tx) })
	s.assertBalance("bob", "USD", "1")
}

// TestFXConversion checks that a conversion balances per currency through the
// FX account.
func (s *Suite) TestFXConversion() {
	rate := ledgo.Rate{From: "USD", To: "EUR", Value: decimal.RequireFromString("0.9137"), Source: "test", Time: time.Now()}
	usd := decimal.RequireFromString("100.05")
	eur, err := rate.Convert(usd)
	s.Require().NoError(err)

	// 100.05 * 0.9137 = 91.415685, rounded half-even to 91.42.
	s.commit(ledgo.Transaction{
		IdempotencyKey: s.name("fx"),
		Rate:           &rate,
		Entries: []ledgo.Entry{
			s.entry("alice", "USD", usd.Neg()),
			s.entry("fx", "USD", usd),
			s.entry("fx", "EUR", eur.Neg()),
			s.entry("bob", "EUR", eur),
		},
	})

	s.assertBalance("alice", "USD", "-100.05")
	s.assertBalance("fx", "USD", "100.05")
	s.assertBalance("fx", "EUR", "-91.42")
	s.assertBalance("bob", "EUR", "91.42")
}

// TestConcurrentTransfers moves money around a ring of accounts in two
// currencies from many goroutines, then checks that debits equal credits per
// currency.
func (s *Suite) TestConcurrentTransfers() {
	accounts := []string{"a", "b", "c", "d"}
	currencies := []string{"USD", "BTC"}
	var next atomic.Int64

	s.parallel(func() error {
		for range transfersPerWorker {
			i := int(next.Add(1))
			from, to := accounts[i%len(accounts)], accounts[(i+1)%len(accounts)]
			currency, amount := currencies[i%2], decimal.New(int64(i%7+1), -2)
			err := s.ledger.Commit(context.Background(), ledgo.Transaction{
				IdempotencyKey: s.name(fmt.Sprint(i)),
				Entries:        []ledgo.Entry{s.entry(from, currency, amount.Neg()), s.entry(to, currency, amount)},
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	for _, currency := range currencies {
		total := decimal.Zero
		for _, account := range accounts {
			total = total.Add(s.balance(account, currency))
		}
		s.Truef(total.IsZero(), "%s: debits and credits differ by %s", currency, total)
	}
}

// name makes account names and keys unique to this run and test.
func (s *Suite) name(base string) string {
	return fmt.Sprintf("%s/%s/%s", s.prefix, s.T().Name(), base)
}

func (s *Suite) entry(account, currency string, amount decimal.Decimal) ledgo.Entry {
	return ledgo.Entry{Account: s.name(account), Currency: currency, Amount: amount}
}

// transfer moves amount from alice to bob.
func (s *Suite) transfer(key, currency, amount string) ledgo.Transaction {
	a := decimal.RequireFromString(amount)
	return ledgo.Transaction{
		IdempotencyKey: s.name(key),
		Entries:        []ledgo.Entry{s.entry("alice", currency, a.Neg()), s.entry("bob", currency, a)},
	}
}

func (s *Suite) commit(tx ledgo.Transaction) {
	s.Require().NoError(s.ledger.Commit(context.Background(), tx), tx.IdempotencyKey)
}

func (s *Suite) balance(account, currency string) decimal.Decimal {
	got, err := s.ledger.Balance(context.Background(), s.name(account), currency)
	s.Require().NoError(err)
	return got
}

func (s *Suite) assertBalance(account, currency, want string) {
	got := s.balance(account, currency)
	s.Truef(got.Equal(decimal.RequireFromString(want)), "Balance(%s, %s) = %s, want %s", account, currency, got, want)
}

// parallel runs fn on several goroutines at once. Errors are collected and
// asserted on the test goroutine, since FailNow must not be called elsewhere.
func (s *Suite) parallel(fn func() error) {
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() { errs <- fn() })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		s.Require().NoError(err)
	}
}
