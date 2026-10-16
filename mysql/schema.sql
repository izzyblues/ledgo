-- Migrate splits this file on semicolons, so keep them out of comments.

-- utf8mb4_0900_bin compares IDs and keys exactly: the default collation is
-- case- and accent-insensitive, and the older utf8mb4_bin ignores trailing
-- spaces. Keys are at most 768 characters, InnoDB's 3072-byte index limit.

CREATE TABLE IF NOT EXISTS ledgo_accounts (
    id                             VARCHAR(768)   NOT NULL PRIMARY KEY,
    currency                       VARCHAR(32)    NOT NULL,
    debits_must_not_exceed_credits BOOLEAN        NOT NULL,
    credits_must_not_exceed_debits BOOLEAN        NOT NULL,
    -- Debits minus credits, cached only for accounts with a limit.
    -- NULL means the balance is summed from entries, so commits never lock the row.
    balance                        DECIMAL(38, 18),
    created_at                     DATETIME(6)    NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS ledgo_transactions (
    id              BIGINT         NOT NULL AUTO_INCREMENT PRIMARY KEY,
    idempotency_key VARCHAR(768)   NOT NULL UNIQUE,
    fingerprint     BINARY(32)     NOT NULL,
    reference       VARCHAR(1000)  NOT NULL,
    rate_from       VARCHAR(32),
    rate_to         VARCHAR(32),
    rate_value      DECIMAL(65, 30),
    rate_source     VARCHAR(1000),
    rate_time       DATETIME(6), -- UTC
    created_at      DATETIME(6)    NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS ledgo_entries (
    id             BIGINT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    transaction_id BIGINT          NOT NULL,
    account        VARCHAR(768)    NOT NULL,
    currency       VARCHAR(32)     NOT NULL,
    amount         DECIMAL(38, 18) NOT NULL,
    FOREIGN KEY (transaction_id) REFERENCES ledgo_transactions (id),
    FOREIGN KEY (account) REFERENCES ledgo_accounts (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_bin;
