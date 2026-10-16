package oracle_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/izzyblues/ledgo"
	"github.com/izzyblues/ledgo/ledgotest"
	"github.com/izzyblues/ledgo/oracle"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// dsn points at the database started by `docker compose up -d`.
const dsn = "oracle://ledgo:ledgo@localhost:1521/FREEPDB1" //nolint:gosec // local Docker credentials

func TestConformance(t *testing.T) {
	_, store := openStore(t)
	suite.Run(t, &ledgotest.Suite{Store: store})
}

// TestInTx runs caller transactions on sessions that use ',' as the decimal
// separator, to show that amounts don't depend on NLS settings.
func TestInTx(t *testing.T) {
	db, store := openStore(t)
	suite.Run(t, &ledgotest.TxSuite{Store: store, Begin: func() (ledgo.Store, func(bool) error, error) {
		return beginWithCommaSeparator(db)
	}})
}

func beginWithCommaSeparator(db *sql.DB) (ledgo.Store, func(bool) error, error) {
	conn, err := db.Conn(context.Background())
	if err != nil {
		return nil, nil, err
	}
	_, err = conn.ExecContext(context.Background(), `ALTER SESSION SET NLS_NUMERIC_CHARACTERS = ',.'`)
	if err != nil {
		return nil, nil, errors.Join(err, conn.Close())
	}
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, nil, errors.Join(err, conn.Close())
	}
	end := func(commit bool) error { return errors.Join(ledgotest.EndTx(tx)(commit), conn.Close()) }
	return oracle.InTx(tx), end, nil
}

func openStore(t *testing.T) (*sql.DB, *oracle.Store) {
	db, err := sql.Open("oracle", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	store := oracle.New(db)
	require.NoError(t, store.Migrate(context.Background()), "is Oracle running? try `docker compose up -d`")
	return db, store
}
