package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/izzyblues/ledgo/ledgotest"
	"github.com/izzyblues/ledgo/postgres"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// defaultDSN points at the database started by `docker compose up -d`.
const defaultDSN = "postgres://ledgo:ledgo@localhost:5432/ledgo?sslmode=disable" //nolint:gosec // local Docker credentials

func TestConformance(t *testing.T) {
	dsn := os.Getenv("LEDGO_POSTGRES_DSN")
	if dsn == "" {
		dsn = defaultDSN
	}
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	store := postgres.New(db)
	require.NoError(t, store.Migrate(context.Background()), "is Postgres running? try `docker compose up -d`")

	suite.Run(t, &ledgotest.Suite{Store: store})
}
