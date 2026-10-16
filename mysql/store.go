// Package mysql is a MySQL ledgo.Store, for InnoDB tables.
package mysql

import (
	"context"
	"database/sql"
	_ "embed"

	"github.com/izzyblues/ledgo"
	"github.com/izzyblues/ledgo/internal/sqlstore"
	"github.com/shopspring/decimal"
)

//go:embed schema.sql
var schema string

// Store keeps accounts and transactions in MySQL.
//
// Commits lock only the accounts that have a balance limit, always in ID
// order, so concurrent commits cannot deadlock on READ COMMITTED and need no
// retry loop.
type Store struct {
	store *sqlstore.Store
}

// New returns a Store on db, opened with sql.Open("mysql", dsn). Each Commit
// runs in its own READ COMMITTED transaction.
func New(db *sql.DB) *Store {
	return &Store{sqlstore.New(db, dialect{})}
}

// InTx returns a Store that writes into tx, so postings commit or roll back
// together with the caller's own changes. The caller commits tx, and must roll
// it back if a Commit fails.
//
// Start tx at READ COMMITTED. Under MySQL's default REPEATABLE READ, a
// duplicate committed by another transaction after tx's first read is
// invisible to tx, so retries can fail instead of being no-ops.
func InTx(tx *sql.Tx) *Store {
	return &Store{sqlstore.InTx(tx, dialect{})}
}

// Migrate creates the ledgo tables if they don't exist. MySQL commits DDL
// implicitly, so call it on a Store from New, never from InTx.
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
