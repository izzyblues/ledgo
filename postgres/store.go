// Package postgres is a PostgreSQL ledgo.Store.
package postgres

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/izzyblues/ledgo"
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
	db *sql.DB // nil when the Store writes into a caller's transaction
	tx *sql.Tx
}

// querier is what *sql.DB and *sql.Tx have in common.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// New returns a Store on db, opened with sql.Open("pgx", dsn). Each Commit
// runs in its own database transaction.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// InTx returns a Store that writes into tx, so postings commit or roll back
// together with the caller's own changes. The caller commits tx, and must roll
// it back if a Commit fails.
func InTx(tx *sql.Tx) *Store {
	return &Store{tx: tx}
}

func (s *Store) querier() querier {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Migrate creates the ledgo tables if they don't exist.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.querier().ExecContext(ctx, schema)
	return err
}

// CreateAccount registers account. Creating the same account again is a
// no-op; with different settings it returns ledgo.ErrAccountConflict.
func (s *Store) CreateAccount(ctx context.Context, account ledgo.Account) error {
	var id string
	err := s.querier().QueryRowContext(ctx, `
		INSERT INTO ledgo_accounts (id, currency, debits_must_not_exceed_credits, credits_must_not_exceed_debits, balance)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
		RETURNING id`,
		account.ID, account.Currency, account.DebitsMustNotExceedCredits, account.CreditsMustNotExceedDebits,
		initialBalance(account)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return checkSameAccount(ctx, s.querier(), account)
	}
	return err
}

// initialBalance is NULL for accounts without a limit: their balance is
// summed from entries instead of cached, so commits never lock them.
func initialBalance(account ledgo.Account) any {
	if !account.HasLimit() {
		return nil
	}
	return decimal.Zero
}

func checkSameAccount(ctx context.Context, q querier, account ledgo.Account) error {
	stored, err := loadAccounts(ctx, q, []string{account.ID})
	if err != nil {
		return err
	}
	if stored[account.ID] != account {
		return ledgo.ErrAccountConflict
	}
	return nil
}

// Commit stores tx. See ledgo.Store.Commit.
func (s *Store) Commit(ctx context.Context, tx ledgo.Transaction) error {
	if s.tx != nil {
		return commit(ctx, s.tx, tx)
	}
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = sqlTx.Rollback() }() // no-op after Commit

	if err := commit(ctx, sqlTx, tx); err != nil {
		return err
	}
	return sqlTx.Commit()
}

func commit(ctx context.Context, q querier, tx ledgo.Transaction) error {
	id, err := insertTransaction(ctx, q, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return checkSameContent(ctx, q, tx)
	}
	if err != nil {
		return err
	}
	accounts, err := loadAccounts(ctx, q, accountIDs(tx.Entries))
	if err != nil {
		return err
	}
	if err := ledgo.CheckEntries(tx.Entries, accounts); err != nil {
		return err
	}
	if err := insertEntries(ctx, q, id, tx.Entries); err != nil {
		return err
	}
	return updateBalances(ctx, q, balanceChanges(tx.Entries, accounts))
}

// insertTransaction returns sql.ErrNoRows if the idempotency key already
// exists. A concurrent insert of the same key blocks until the other
// transaction commits or rolls back.
func insertTransaction(ctx context.Context, q querier, tx ledgo.Transaction) (int64, error) {
	args := append([]any{tx.IdempotencyKey, tx.Fingerprint(), tx.Reference}, rateColumns(tx.Rate)...)
	var id int64
	err := q.QueryRowContext(ctx, `
		INSERT INTO ledgo_transactions (idempotency_key, fingerprint, reference, rate_from, rate_to, rate_value, rate_source, rate_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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

func checkSameContent(ctx context.Context, q querier, tx ledgo.Transaction) error {
	var stored []byte
	err := q.QueryRowContext(ctx,
		`SELECT fingerprint FROM ledgo_transactions WHERE idempotency_key = $1`, tx.IdempotencyKey).Scan(&stored)
	if err != nil {
		return err
	}
	if !bytes.Equal(stored, tx.Fingerprint()) {
		return ledgo.ErrIdempotencyConflict
	}
	return nil
}

func accountIDs(entries []ledgo.Entry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.Account
	}
	return ids
}

func loadAccounts(ctx context.Context, q querier, ids []string) (map[string]ledgo.Account, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, currency, debits_must_not_exceed_credits, credits_must_not_exceed_debits
		FROM ledgo_accounts
		WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	accounts := map[string]ledgo.Account{}
	for rows.Next() {
		var a ledgo.Account
		if err := rows.Scan(&a.ID, &a.Currency, &a.DebitsMustNotExceedCredits, &a.CreditsMustNotExceedDebits); err != nil {
			return nil, err
		}
		accounts[a.ID] = a
	}
	return accounts, rows.Err()
}

// ponytail: one INSERT per entry; batch with unnest() if large transactions get slow.
func insertEntries(ctx context.Context, q querier, transactionID int64, entries []ledgo.Entry) error {
	for _, e := range entries {
		_, err := q.ExecContext(ctx,
			`INSERT INTO ledgo_entries (transaction_id, account, currency, amount) VALUES ($1, $2, $3, $4)`,
			transactionID, e.Account, e.Currency, e.Amount)
		if err != nil {
			return err
		}
	}
	return nil
}

type balanceChange struct {
	account ledgo.Account
	amount  decimal.Decimal
}

// balanceChanges nets the entries per account, for accounts with a limit
// only. The others keep no cached balance.
func balanceChanges(entries []ledgo.Entry, accounts map[string]ledgo.Account) map[string]balanceChange {
	changes := map[string]balanceChange{}
	for _, e := range entries {
		account := accounts[e.Account]
		if account.HasLimit() {
			changes[e.Account] = balanceChange{account: account, amount: changes[e.Account].amount.Add(e.Amount)}
		}
	}
	return changes
}

// updateBalances locks and updates accounts in ID order, so two commits
// touching the same accounts always lock them in the same order and cannot
// deadlock.
func updateBalances(ctx context.Context, q querier, changes map[string]balanceChange) error {
	for _, id := range slices.Sorted(maps.Keys(changes)) {
		change := changes[id]
		var balance decimal.Decimal
		err := q.QueryRowContext(ctx,
			`UPDATE ledgo_accounts SET balance = balance + $2 WHERE id = $1 RETURNING balance`,
			id, change.amount).Scan(&balance)
		if err != nil {
			return err
		}
		if err := change.account.CheckBalance(balance); err != nil {
			return err
		}
	}
	return nil
}

// Balance is the account's debits minus credits: cached for accounts with a
// limit, summed from entries otherwise.
func (s *Store) Balance(ctx context.Context, account string) (decimal.Decimal, error) {
	var balance decimal.Decimal
	err := s.querier().QueryRowContext(ctx, `
		SELECT COALESCE(a.balance, (SELECT COALESCE(SUM(e.amount), 0) FROM ledgo_entries e WHERE e.account = a.id))
		FROM ledgo_accounts a
		WHERE a.id = $1`, account).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return decimal.Decimal{}, fmt.Errorf("%w: %s", ledgo.ErrUnknownAccount, account)
	}
	return balance, err
}
