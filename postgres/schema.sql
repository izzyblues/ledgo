-- Migrate splits this file on semicolons, so keep them out of comments.

CREATE TABLE IF NOT EXISTS ledgo_accounts (
    id                             TEXT PRIMARY KEY,
    currency                       TEXT        NOT NULL,
    debits_must_not_exceed_credits BOOLEAN     NOT NULL,
    credits_must_not_exceed_debits BOOLEAN     NOT NULL,
    -- Debits minus credits, cached only for accounts with a limit.
    -- NULL means the balance is summed from entries, so commits never lock the row.
    balance                        NUMERIC,
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ledgo_transactions (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    idempotency_key TEXT        NOT NULL UNIQUE,
    fingerprint     BYTEA       NOT NULL,
    reference       TEXT        NOT NULL,
    rate_from       TEXT,
    rate_to         TEXT,
    rate_value      NUMERIC,
    rate_source     TEXT,
    rate_time       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ledgo_entries (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transaction_id BIGINT  NOT NULL REFERENCES ledgo_transactions (id),
    account        TEXT    NOT NULL REFERENCES ledgo_accounts (id),
    currency       TEXT    NOT NULL,
    amount         NUMERIC NOT NULL
);

CREATE INDEX IF NOT EXISTS ledgo_entries_account ON ledgo_entries (account);
