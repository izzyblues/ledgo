// Package sqlstore is the database/sql ledgo.Store shared by the SQL
// backends. Each backend supplies a Dialect and its schema.
package sqlstore

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/izzyblues/ledgo"
	"github.com/shopspring/decimal"
)

// Store keeps accounts and transactions in a SQL database.
//
// Commits lock only the accounts that have a balance limit, always in ID
// order, so concurrent commits cannot deadlock on READ COMMITTED and need no
// retry loop.
type Store struct {
	db      *sql.DB // nil when the Store writes into a caller's transaction
	tx      *sql.Tx
	dialect Dialect
}

// New returns a Store on db. Each Commit runs in its own READ COMMITTED
// transaction, started with the dialect's TxOptions.
func New(db *sql.DB, dialect Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// InTx returns a Store that writes into tx, so postings commit or roll back
// together with the caller's own changes.
func InTx(tx *sql.Tx, dialect Dialect) *Store {
	return &Store{tx: tx, dialect: dialect}
}

// conn runs the store's queries on q, rewriting their "?" placeholders for
// the dialect.
type conn struct {
	q       querier
	dialect Dialect
}

// querier is what *sql.DB and *sql.Tx have in common.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) conn() conn {
	if s.tx != nil {
		return conn{q: s.tx, dialect: s.dialect}
	}
	return conn{q: s.db, dialect: s.dialect}
}

func (c conn) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.q.ExecContext(ctx, c.rebind(query), args...)
}

func (c conn) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.q.QueryContext(ctx, c.rebind(query), args...)
}

func (c conn) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return c.q.QueryRowContext(ctx, c.rebind(query), args...)
}

// rebind replaces each "?" with the dialect's placeholder. The store's
// queries contain no other "?".
func (c conn) rebind(query string) string {
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r != '?' {
			b.WriteRune(r)
			continue
		}
		n++
		b.WriteString(c.dialect.Placeholder(n))
	}
	return b.String()
}

// Migrate runs schema, a list of statements separated by ";", skipping
// tables and indexes that already exist.
func (s *Store) Migrate(ctx context.Context, schema string) error {
	c := s.conn()
	for statement := range strings.SplitSeq(schema, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		_, err := c.q.ExecContext(ctx, statement)
		if err != nil && !s.dialect.AlreadyExists(err) {
			return err
		}
	}
	return nil
}

// CreateAccount registers account. Creating the same account again is a
// no-op; with different settings it returns ledgo.ErrAccountConflict.
func (s *Store) CreateAccount(ctx context.Context, account ledgo.Account) error {
	if err := s.conn().createAccount(ctx, account); err != nil {
		return fmt.Errorf("create account %q: %w", account.ID, err)
	}
	return nil
}

func (c conn) createAccount(ctx context.Context, account ledgo.Account) error {
	inserted, err := c.insertUnique(ctx, `
		INSERT INTO ledgo_accounts (id, currency, debits_must_not_exceed_credits, credits_must_not_exceed_debits, balance)
		VALUES (?, ?, ?, ?, `+c.dialect.Number("?")+`)`,
		account.ID, account.Currency, account.DebitsMustNotExceedCredits, account.CreditsMustNotExceedDebits,
		initialBalance(account))
	if err != nil || inserted {
		return err
	}
	return c.checkSameAccount(ctx, account)
}

// insertUnique runs a single-row INSERT and reports false, with the
// transaction still usable, if the row's unique key already exists. A
// concurrent insert of the same key blocks until the other transaction
// commits or rolls back.
func (c conn) insertUnique(ctx context.Context, insert string, args ...any) (bool, error) {
	result, err := c.exec(ctx, c.dialect.IgnoreDuplicate(insert), args...)
	if c.dialect.IsDuplicate(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// initialBalance is NULL for accounts without a limit: their balance is
// summed from entries instead of cached, so commits never lock them.
func initialBalance(account ledgo.Account) any {
	if !account.HasLimit() {
		return nil
	}
	return decimal.Zero
}

func (c conn) checkSameAccount(ctx context.Context, account ledgo.Account) error {
	stored, err := c.loadAccounts(ctx, []string{account.ID})
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
	if err := s.commit(ctx, tx); err != nil {
		return fmt.Errorf("commit %q: %w", tx.IdempotencyKey, err)
	}
	return nil
}

func (s *Store) commit(ctx context.Context, tx ledgo.Transaction) error {
	if s.tx != nil {
		return s.conn().commit(ctx, tx)
	}
	sqlTx, err := s.db.BeginTx(ctx, s.dialect.TxOptions())
	if err != nil {
		return err
	}
	defer func() { _ = sqlTx.Rollback() }() // no-op after Commit

	if err := (conn{q: sqlTx, dialect: s.dialect}).commit(ctx, tx); err != nil {
		return err
	}
	return sqlTx.Commit()
}

func (c conn) commit(ctx context.Context, tx ledgo.Transaction) error {
	inserted, err := c.insertTransaction(ctx, tx)
	if err != nil {
		return err
	}
	id, fingerprint, err := c.storedTransaction(ctx, tx.IdempotencyKey)
	if err != nil {
		return err
	}
	if !inserted {
		return checkSameContent(fingerprint, tx)
	}
	accounts, err := c.loadAccounts(ctx, accountIDs(tx.Entries))
	if err != nil {
		return err
	}
	if err := ledgo.CheckEntries(tx.Entries, accounts); err != nil {
		return err
	}
	// Balances before entries: InnoDB share-locks the accounts that new
	// entries reference, so locking them for the update afterwards could
	// deadlock with a concurrent commit.
	if err := c.updateBalances(ctx, balanceChanges(tx.Entries, accounts)); err != nil {
		return err
	}
	return c.insertEntries(ctx, id, tx.Entries)
}

func (c conn) insertTransaction(ctx context.Context, tx ledgo.Transaction) (bool, error) {
	args := append([]any{tx.IdempotencyKey, tx.Fingerprint(), tx.Reference}, rateColumns(tx.Rate)...)
	return c.insertUnique(ctx, `
		INSERT INTO ledgo_transactions (idempotency_key, fingerprint, reference, rate_from, rate_to, rate_value, rate_source, rate_time)
		VALUES (?, ?, ?, ?, ?, `+c.dialect.Number("?")+`, ?, ?)`, args...)
}

func rateColumns(r *ledgo.Rate) []any {
	if r == nil {
		return []any{nil, nil, nil, nil, nil}
	}
	return []any{r.From, r.To, r.Value, r.Source, r.Time}
}

// ponytail: a second round trip for the ID, so every dialect shares one
// insert path; use RETURNING or LastInsertId if commits get slow.
func (c conn) storedTransaction(ctx context.Context, key string) (id int64, fingerprint []byte, err error) {
	err = c.queryRow(ctx,
		`SELECT id, fingerprint FROM ledgo_transactions WHERE idempotency_key = ?`, key).Scan(&id, &fingerprint)
	return id, fingerprint, err
}

func checkSameContent(stored []byte, tx ledgo.Transaction) error {
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

// ponytail: one bind per ID, and Oracle allows at most 1000 in an IN list;
// split the list if transactions ever touch more accounts.
func (c conn) loadAccounts(ctx context.Context, ids []string) (map[string]ledgo.Account, error) {
	accounts := map[string]ledgo.Account{}
	if len(ids) == 0 {
		return accounts, nil // IN () is invalid SQL
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := c.query(ctx, `
		SELECT id, currency, debits_must_not_exceed_credits, credits_must_not_exceed_debits
		FROM ledgo_accounts
		WHERE id IN (`+strings.Repeat("?, ", len(ids)-1)+`?)`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var a ledgo.Account
		if err := rows.Scan(&a.ID, &a.Currency, &a.DebitsMustNotExceedCredits, &a.CreditsMustNotExceedDebits); err != nil {
			return nil, err
		}
		accounts[a.ID] = a
	}
	return accounts, rows.Err()
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
// deadlock. The balance is read back after the UPDATE, which already holds
// the row lock.
func (c conn) updateBalances(ctx context.Context, changes map[string]balanceChange) error {
	for _, id := range slices.Sorted(maps.Keys(changes)) {
		change := changes[id]
		_, err := c.exec(ctx,
			`UPDATE ledgo_accounts SET balance = balance + `+c.dialect.Number("?")+` WHERE id = ?`, change.amount, id)
		if err != nil {
			return err
		}
		var balance decimal.Decimal
		err = c.queryRow(ctx, `SELECT `+c.dialect.Text("balance")+` FROM ledgo_accounts WHERE id = ?`, id).Scan(&balance)
		if err != nil {
			return err
		}
		if err := change.account.CheckBalance(balance); err != nil {
			return err
		}
	}
	return nil
}

// ponytail: one INSERT per entry; batch them if large transactions get slow.
func (c conn) insertEntries(ctx context.Context, transactionID int64, entries []ledgo.Entry) error {
	for _, e := range entries {
		_, err := c.exec(ctx,
			`INSERT INTO ledgo_entries (transaction_id, account, currency, amount) VALUES (?, ?, ?, `+c.dialect.Number("?")+`)`,
			transactionID, e.Account, e.Currency, e.Amount)
		if err != nil {
			return err
		}
	}
	return nil
}

// Balance is the account's debits minus credits: cached for accounts with a
// limit, summed from entries otherwise.
func (s *Store) Balance(ctx context.Context, account string) (decimal.Decimal, error) {
	var balance decimal.Decimal
	err := s.conn().queryRow(ctx, `
		SELECT `+s.dialect.Text(`COALESCE(a.balance, (SELECT COALESCE(SUM(e.amount), 0) FROM ledgo_entries e WHERE e.account = a.id))`)+`
		FROM ledgo_accounts a
		WHERE a.id = ?`, account).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		err = ledgo.ErrUnknownAccount
	}
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("balance %q: %w", account, err)
	}
	return balance, nil
}
