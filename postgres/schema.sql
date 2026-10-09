CREATE TABLE IF NOT EXISTS ledgo_transactions (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    idempotency_key TEXT        NOT NULL UNIQUE,
    fingerprint     BYTEA       NOT NULL,
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
    account        TEXT    NOT NULL,
    currency       TEXT    NOT NULL,
    amount         NUMERIC NOT NULL
);

CREATE INDEX IF NOT EXISTS ledgo_entries_account_currency ON ledgo_entries (account, currency);
