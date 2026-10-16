package mysql_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/izzyblues/ledgo"
	"github.com/izzyblues/ledgo/ledgotest"
	"github.com/izzyblues/ledgo/mysql"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// dsn points at the database started by `docker compose up -d`. It inlines
// parameters, the mode in which a missing DECIMAL cast loses digits.
const dsn = "ledgo:ledgo@tcp(localhost:3306)/ledgo?interpolateParams=true" //nolint:gosec // local Docker credentials

func TestConformance(t *testing.T) {
	_, store := openStore(t)
	suite.Run(t, &ledgotest.Suite{Store: store})
}

// TestInTx starts caller transactions at READ COMMITTED, as InTx asks.
func TestInTx(t *testing.T) {
	db, store := openStore(t)
	suite.Run(t, &ledgotest.TxSuite{Store: store, Begin: func() (ledgo.Store, func(bool) error, error) {
		tx, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return nil, nil, err
		}
		return mysql.InTx(tx), ledgotest.EndTx(tx), nil
	}})
}

// TestIDsCompareExactly checks that account IDs differing only in case or
// trailing spaces are different accounts, unlike in MySQL's default collation.
func TestIDsCompareExactly(t *testing.T) {
	_, store := openStore(t)
	id := fmt.Sprintf("%s/%d/account", t.Name(), time.Now().UnixNano())
	for _, account := range []ledgo.Account{
		{ID: id, Currency: "USD"},
		{ID: strings.ToUpper(id), Currency: "EUR"},
		{ID: id + " ", Currency: "GBP"},
	} {
		require.NoError(t, store.CreateAccount(context.Background(), account), account.ID)
	}
}

func openStore(t *testing.T) (*sql.DB, *mysql.Store) {
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	store := mysql.New(db)
	require.NoError(t, store.Migrate(context.Background()), "is MySQL running? try `docker compose up -d`")
	return db, store
}
