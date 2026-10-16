// Package oracle is an Oracle Database ledgo.Store.
package oracle

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/izzyblues/ledgo"
	"github.com/shopspring/decimal"
	_ "github.com/sijms/go-ora/v2" // registers the "oracle" database/sql driver
	"github.com/sijms/go-ora/v2/network"
)

//go:embed schema.sql
var schema string

// Oracle error codes.
const (
	uniqueViolation = 1    // ORA-00001: unique constraint violated
	nameInUse       = 955  // ORA-00955: name is already used by an existing object
	alreadyIndexed  = 1408 // ORA-01408: such column list already indexed
)

// Store keeps accounts and transactions in Oracle Database.
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

// New returns a Store on db, opened with sql.Open("oracle", dsn). Each Commit
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

// Migrate creates the ledgo tables if they don't exist. Oracle commits DDL
// implicitly, so call it on a Store from New, never from InTx.
func (s *Store) Migrate(ctx context.Context) error {
	for statement := range strings.SplitSeq(schema, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		_, err := s.querier().ExecContext(ctx, statement)
		if err != nil && !isOracleError(err, nameInUse, alreadyIndexed) {
			return err
		}
	}
	return nil
}

func isOracleError(err error, codes ...int) bool {
	var oracleErr *network.OracleError
	return errors.As(err, &oracleErr) && slices.Contains(codes, oracleErr.ErrCode)
}

// number parses the decimal bound to param. shopspring/decimal always formats
// with a '.' separator, so the separator is set explicitly instead of taken
// from the session's NLS settings.
func number(param string) string {
	return fmt.Sprintf("TO_NUMBER(%s, '%s', %s)", param, numberFormat, decimalPoint)
}

// text formats a NUMBER for scanning into a decimal.Decimal, so it never goes
// through float64.
func text(expr string) string {
	return fmt.Sprintf("TO_CHAR(%s, 'TM9', %s)", expr, decimalPoint)
}

const decimalPoint = `'NLS_NUMERIC_CHARACTERS=''.,'''`

// numberFormat accepts the 20 integer digits of a NUMBER(38, 18) amount, and
// up to 38 decimal places for rates. Oracle rejects longer format models.
var numberFormat = strings.Repeat("9", 20) + "D" + strings.Repeat("9", 38)

// CreateAccount registers account. Creating the same account again is a
// no-op; with different settings it returns ledgo.ErrAccountConflict.
func (s *Store) CreateAccount(ctx context.Context, account ledgo.Account) error {
	_, err := s.querier().ExecContext(ctx, `
		INSERT INTO ledgo_accounts (id, currency, debits_must_not_exceed_credits, credits_must_not_exceed_debits, balance)
		VALUES (:1, :2, :3, :4, `+number(":5")+`)`,
		account.ID, account.Currency, flag(account.DebitsMustNotExceedCredits), flag(account.CreditsMustNotExceedDebits),
		initialBalance(account))
	if isOracleError(err, uniqueViolation) {
		return checkSameAccount(ctx, s.querier(), account)
	}
	return err
}

// flag stores a bool in a NUMBER(1), since BOOLEAN needs Oracle 23ai.
func flag(b bool) int {
	if b {
		return 1
	}
	return 0
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
	if isOracleError(err, uniqueViolation) {
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

// insertTransaction fails with ORA-00001 if the idempotency key already
// exists. A concurrent insert of the same key blocks until the other
// transaction commits or rolls back. Oracle rolls back only the failed
// statement, so the transaction stays usable.
func insertTransaction(ctx context.Context, q querier, tx ledgo.Transaction) (int64, error) {
	var id int64
	args := append([]any{tx.IdempotencyKey, tx.Fingerprint(), tx.Reference}, rateColumns(tx.Rate)...)
	_, err := q.ExecContext(ctx, `
		INSERT INTO ledgo_transactions (idempotency_key, fingerprint, reference, rate_from, rate_to, rate_value, rate_source, rate_time)
		VALUES (:1, :2, :3, :4, :5, `+number(":6")+`, :7, :8)
		RETURNING id INTO :9`, append(args, sql.Out{Dest: &id})...)
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
		`SELECT fingerprint FROM ledgo_transactions WHERE idempotency_key = :1`, tx.IdempotencyKey).Scan(&stored)
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

// ponytail: one bind per ID, and Oracle allows at most 1000 in an IN list;
// split the list if transactions ever touch more accounts.
func loadAccounts(ctx context.Context, q querier, ids []string) (map[string]ledgo.Account, error) {
	params := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		params[i], args[i] = fmt.Sprintf(":%d", i+1), id
	}
	rows, err := q.QueryContext(ctx, `
		SELECT id, currency, debits_must_not_exceed_credits, credits_must_not_exceed_debits
		FROM ledgo_accounts
		WHERE id IN (`+strings.Join(params, ", ")+`)`, args...)
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

// ponytail: one INSERT per entry; batch with array binds if large transactions get slow.
func insertEntries(ctx context.Context, q querier, transactionID int64, entries []ledgo.Entry) error {
	for _, e := range entries {
		_, err := q.ExecContext(ctx,
			`INSERT INTO ledgo_entries (transaction_id, account, currency, amount) VALUES (:1, :2, :3, `+number(":4")+`)`,
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
// deadlock. The balance is read back after the UPDATE, which already holds
// the row lock.
func updateBalances(ctx context.Context, q querier, changes map[string]balanceChange) error {
	for _, id := range slices.Sorted(maps.Keys(changes)) {
		change := changes[id]
		_, err := q.ExecContext(ctx,
			`UPDATE ledgo_accounts SET balance = balance + `+number(":1")+` WHERE id = :2`, change.amount, id)
		if err != nil {
			return err
		}
		var balance decimal.Decimal
		err = q.QueryRowContext(ctx, `SELECT `+text("balance")+` FROM ledgo_accounts WHERE id = :1`, id).Scan(&balance)
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
		SELECT `+text(`COALESCE(a.balance, (SELECT COALESCE(SUM(e.amount), 0) FROM ledgo_entries e WHERE e.account = a.id))`)+`
		FROM ledgo_accounts a
		WHERE a.id = :1`, account).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return decimal.Decimal{}, fmt.Errorf("%w: %s", ledgo.ErrUnknownAccount, account)
	}
	return balance, err
}
