package oracle_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/izzyblues/ledgo"
	"github.com/izzyblues/ledgo/ledgotest"
	"github.com/izzyblues/ledgo/oracle"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// dsn points at the database started by `docker compose up -d`.
const dsn = "oracle://ledgo:ledgo@localhost:1521/FREEPDB1" //nolint:gosec // local Docker credentials

func TestConformance(t *testing.T) {
	_, store := openStore(t)
	suite.Run(t, &ledgotest.Suite{Store: store})
}

func TestInTxCommitsWithCallerTransaction(t *testing.T) {
	db, store := openStore(t)
	bank, customer := createAccounts(t, store)

	commitInTx(t, db, fund(bank, customer), (*sql.Tx).Commit)

	balance, err := store.Balance(context.Background(), customer.ID)
	require.NoError(t, err)
	assert.True(t, balance.Equal(decimal.NewFromInt(-1)), "balance = %s", balance)
}

func TestInTxRollsBackWithCallerTransaction(t *testing.T) {
	db, store := openStore(t)
	bank, customer := createAccounts(t, store)

	commitInTx(t, db, fund(bank, customer), (*sql.Tx).Rollback)

	balance, err := store.Balance(context.Background(), customer.ID)
	require.NoError(t, err)
	assert.True(t, balance.IsZero(), "balance = %s", balance)
}

// TestExactAmounts checks that amounts at the column's full precision round
// trip without going through float64, whatever the session's decimal
// separator.
func TestExactAmounts(t *testing.T) {
	db, store := openStore(t)
	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	_, err = conn.ExecContext(context.Background(), `ALTER SESSION SET NLS_NUMERIC_CHARACTERS = ',.'`)
	require.NoError(t, err)

	sqlTx, err := conn.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	inTx := oracle.InTx(sqlTx)
	ledgo.RegisterCurrency("ETH", 18)
	prefix := fmt.Sprintf("%s/%d", t.Name(), time.Now().UnixNano())
	from := ledgo.Account{ID: prefix + "/from", Currency: "ETH"}
	to := ledgo.Account{ID: prefix + "/to", Currency: "ETH", CreditsMustNotExceedDebits: true}
	require.NoError(t, inTx.CreateAccount(context.Background(), from))
	require.NoError(t, inTx.CreateAccount(context.Background(), to))

	amount := decimal.RequireFromString("12345678901234567890.123456789012345678")
	tx := ledgo.NewTransaction(prefix+"/tx", "").Debit(to, amount).Credit(from, amount).Build()
	require.NoError(t, ledgo.New(inTx).Commit(context.Background(), tx))
	require.NoError(t, sqlTx.Commit())

	for account, want := range map[string]decimal.Decimal{from.ID: amount.Neg(), to.ID: amount} {
		got, err := store.Balance(context.Background(), account)
		require.NoError(t, err)
		assert.True(t, got.Equal(want), "Balance(%s) = %s, want %s", account, got, want)
	}
}

func openStore(t *testing.T) (*sql.DB, *oracle.Store) {
	db, err := sql.Open("oracle", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	store := oracle.New(db)
	require.NoError(t, store.Migrate(context.Background()), "is Oracle running? try `docker compose up -d`")
	return db, store
}

func createAccounts(t *testing.T, store *oracle.Store) (bank, customer ledgo.Account) {
	prefix := fmt.Sprintf("%s/%d", t.Name(), time.Now().UnixNano())
	bank = ledgo.Account{ID: prefix + "/bank", Currency: "USD"}
	customer = ledgo.Account{ID: prefix + "/customer", Currency: "USD", DebitsMustNotExceedCredits: true}
	require.NoError(t, store.CreateAccount(context.Background(), bank))
	require.NoError(t, store.CreateAccount(context.Background(), customer))
	return bank, customer
}

// fund moves 1 USD from the bank to the customer.
func fund(bank, customer ledgo.Account) ledgo.Transaction {
	return ledgo.Transaction{
		IdempotencyKey: customer.ID + "/fund",
		Entries: []ledgo.Entry{
			{Account: bank.ID, Currency: "USD", Amount: decimal.NewFromInt(1)},
			{Account: customer.ID, Currency: "USD", Amount: decimal.NewFromInt(-1)},
		},
	}
}

// commitInTx commits tx through a Store bound to a caller's transaction, then
// ends that transaction with end.
func commitInTx(t *testing.T, db *sql.DB, tx ledgo.Transaction, end func(*sql.Tx) error) {
	sqlTx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, ledgo.New(oracle.InTx(sqlTx)).Commit(context.Background(), tx))
	require.NoError(t, end(sqlTx))
}
