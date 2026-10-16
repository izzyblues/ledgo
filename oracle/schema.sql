-- Migrate splits this file on semicolons, so keep them out of comments.

CREATE TABLE ledgo_accounts (
    id                             VARCHAR2(1000 CHAR) PRIMARY KEY,
    currency                       VARCHAR2(32 CHAR)        NOT NULL,
    debits_must_not_exceed_credits NUMBER(1)                NOT NULL CHECK (debits_must_not_exceed_credits IN (0, 1)),
    credits_must_not_exceed_debits NUMBER(1)                NOT NULL CHECK (credits_must_not_exceed_debits IN (0, 1)),
    -- Debits minus credits, cached only for accounts with a limit.
    -- NULL means the balance is summed from entries, so commits never lock the row.
    balance                        NUMBER(38, 18),
    created_at                     TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL
);

CREATE TABLE ledgo_transactions (
    id              NUMBER(19) GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    idempotency_key VARCHAR2(1000 CHAR)      NOT NULL UNIQUE,
    fingerprint     RAW(32)                  NOT NULL,
    -- Nullable because Oracle stores the empty string as NULL.
    reference       VARCHAR2(1000 CHAR),
    rate_from       VARCHAR2(32 CHAR),
    rate_to         VARCHAR2(32 CHAR),
    rate_value      NUMBER,
    rate_source     VARCHAR2(1000 CHAR),
    rate_time       TIMESTAMP WITH TIME ZONE,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL
);

CREATE TABLE ledgo_entries (
    id             NUMBER(19) GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transaction_id NUMBER(19)          NOT NULL REFERENCES ledgo_transactions (id),
    account        VARCHAR2(1000 CHAR) NOT NULL REFERENCES ledgo_accounts (id),
    currency       VARCHAR2(32 CHAR)   NOT NULL,
    amount         NUMBER(38, 18)      NOT NULL
);

CREATE INDEX ledgo_entries_account ON ledgo_entries (account);

CREATE INDEX ledgo_entries_transaction ON ledgo_entries (transaction_id);
