package postgres_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/izzyblues/ledgo"
	"github.com/izzyblues/ledgo/ledgotest"
	"github.com/izzyblues/ledgo/postgres"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// dsn points at the database started by `docker compose up -d`.
const dsn = "postgres://ledgo:ledgo@localhost:5432/ledgo?sslmode=disable" //nolint:gosec // local Docker credentials

func TestConformance(t *testing.T) {
	_, store := openStore(t)
	suite.Run(t, &ledgotest.Suite{Store: store})
}

func TestInTx(t *testing.T) {
	db, store := openStore(t)
	suite.Run(t, &ledgotest.TxSuite{Store: store, Begin: func() (ledgo.Store, func(bool) error, error) {
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			return nil, nil, err
		}
		return postgres.InTx(tx), ledgotest.EndTx(tx), nil
	}})
}

func openStore(t *testing.T) (*sql.DB, *postgres.Store) {
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	store := postgres.New(db)
	require.NoError(t, store.Migrate(context.Background()), "is Postgres running? try `docker compose up -d`")
	return db, store
}
