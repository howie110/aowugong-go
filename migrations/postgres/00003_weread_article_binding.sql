-- +goose Up

CREATE TABLE IF NOT EXISTS weread_article_credential (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    ciphertext BYTEA NOT NULL,
    encryption_version SMALLINT NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS'))
);

CREATE TABLE IF NOT EXISTS weread_article_account (
    account_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    cover_url TEXT NOT NULL DEFAULT '',
    is_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    discovered_at TEXT NOT NULL DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT NOT NULL DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS'))
);
CREATE INDEX IF NOT EXISTS idx_weread_article_account_enabled
    ON weread_article_account(is_enabled, account_id);

-- +goose Down

DROP TABLE IF EXISTS weread_article_account;
DROP TABLE IF EXISTS weread_article_credential;
