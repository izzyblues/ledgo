// Package postgres is a PostgreSQL ledgo.Store.
package postgres

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"errors"

	"github.com/izzyblues/ledgo"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/shopspring/decimal"
)

//go:embed schema.sql
var schema string

// Store keeps transactions in PostgreSQL. Balances are summed from entries.
type Store struct {
	db *sql.DB
}

// New returns a Store on db, opened with sql.Open("pgx", dsn).
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates the ledgo tables if they don't exist.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Commit takes no row locks beyond its own idempotency key and runs on READ
// COMMITTED, so it cannot deadlock and needs no retry loop.
func (s *Store) Commit(ctx context.Context, tx ledgo.Transaction) error {
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = sqlTx.Rollback() }() // no-op after Commit

	id, err := insertTransaction(ctx, sqlTx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return checkSameContent(ctx, sqlTx, tx)
	}
	if err != nil {
		return err
	}
	if err := insertEntries(ctx, sqlTx, id, tx.Entries); err != nil {
		return err
	}
	return sqlTx.Commit()
}

// insertTransaction returns sql.ErrNoRows if the idempotency key already
// exists. A concurrent insert of the same key blocks until the other
// transaction commits or rolls back.
func insertTransaction(ctx context.Context, sqlTx *sql.Tx, tx ledgo.Transaction) (int64, error) {
	args := append([]any{tx.IdempotencyKey, tx.Fingerprint()}, rateColumns(tx.Rate)...)
	var id int64
	err := sqlTx.QueryRowContext(ctx, `
		INSERT INTO ledgo_transactions (idempotency_key, fingerprint, rate_from, rate_to, rate_value, rate_source, rate_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id`, args...).Scan(&id)
	return id, err
}

func rateColumns(r *ledgo.Rate) []any {
	if r == nil {
		return []any{nil, nil, nil, nil, nil}
	}
	return []any{r.From, r.To, r.Value, r.Source, r.Time}
}

func checkSameContent(ctx context.Context, sqlTx *sql.Tx, tx ledgo.Transaction) error {
	var stored []byte
	err := sqlTx.QueryRowContext(ctx,
		`SELECT fingerprint FROM ledgo_transactions WHERE idempotency_key = $1`, tx.IdempotencyKey).Scan(&stored)
	if err != nil {
		return err
	}
	if !bytes.Equal(stored, tx.Fingerprint()) {
		return ledgo.ErrIdempotencyConflict
	}
	return nil
}

// ponytail: one INSERT per entry; batch with unnest() if large transactions get slow.
func insertEntries(ctx context.Context, sqlTx *sql.Tx, transactionID int64, entries []ledgo.Entry) error {
	for _, e := range entries {
		_, err := sqlTx.ExecContext(ctx,
			`INSERT INTO ledgo_entries (transaction_id, account, currency, amount) VALUES ($1, $2, $3, $4)`,
			transactionID, e.Account, e.Currency, e.Amount)
		if err != nil {
			return err
		}
	}
	return nil
}

// Balance sums the account's entries in currency.
func (s *Store) Balance(ctx context.Context, account, currency string) (decimal.Decimal, error) {
	var balance decimal.Decimal
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM ledgo_entries WHERE account = $1 AND currency = $2`,
		account, currency).Scan(&balance)
	return balance, err
}
