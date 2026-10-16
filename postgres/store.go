// Package postgres is a PostgreSQL ledgo.Store.
package postgres

import (
	"context"
	"database/sql"
	_ "embed"

	"github.com/izzyblues/ledgo"
	"github.com/izzyblues/ledgo/internal/sqlstore"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/shopspring/decimal"
)

//go:embed schema.sql
var schema string

// Store keeps accounts and transactions in PostgreSQL.
//
// Commits lock only the accounts that have a balance limit, always in ID
// order, so concurrent commits cannot deadlock on READ COMMITTED and need no
// retry loop.
type Store struct {
	store *sqlstore.Store
}

// New returns a Store on db, opened with sql.Open("pgx", dsn). Each Commit
// runs in its own database transaction.
func New(db *sql.DB) *Store {
	return &Store{sqlstore.New(db, dialect{})}
}

// InTx returns a Store that writes into tx, so postings commit or roll back
// together with the caller's own changes. The caller commits tx, and must roll
// it back if a Commit fails.
func InTx(tx *sql.Tx) *Store {
	return &Store{sqlstore.InTx(tx, dialect{})}
}

// Migrate creates the ledgo tables if they don't exist.
func (s *Store) Migrate(ctx context.Context) error {
	return s.store.Migrate(ctx, schema)
}

// CreateAccount registers account. Creating the same account again is a
// no-op; with different settings it returns ledgo.ErrAccountConflict.
func (s *Store) CreateAccount(ctx context.Context, account ledgo.Account) error {
	return s.store.CreateAccount(ctx, account)
}

// Commit stores tx. See ledgo.Store.Commit.
func (s *Store) Commit(ctx context.Context, tx ledgo.Transaction) error {
	return s.store.Commit(ctx, tx)
}

// Balance is the account's debits minus credits: cached for accounts with a
// limit, summed from entries otherwise.
func (s *Store) Balance(ctx context.Context, account string) (decimal.Decimal, error) {
	return s.store.Balance(ctx, account)
}
