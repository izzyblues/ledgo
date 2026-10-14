package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/izzyblues/ledgo"
	"github.com/izzyblues/ledgo/ledgotest"
	"github.com/izzyblues/ledgo/postgres"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// dsn points at the database started by `docker compose up -d`.
const dsn = "postgres://ledgo:ledgo@localhost:5432/ledgo?sslmode=disable" //nolint:gosec // local Docker credentials

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

func openStore(t *testing.T) (*sql.DB, *postgres.Store) {
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	store := postgres.New(db)
	require.NoError(t, store.Migrate(context.Background()), "is Postgres running? try `docker compose up -d`")
	return db, store
}

func createAccounts(t *testing.T, store *postgres.Store) (bank, customer ledgo.Account) {
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
	require.NoError(t, ledgo.New(postgres.InTx(sqlTx)).Commit(context.Background(), tx))
	require.NoError(t, end(sqlTx))
}
