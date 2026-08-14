-- +goose Up

CREATE TABLE IF NOT EXISTS weread_article_credential (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    ciphertext BLOB NOT NULL,
    encryption_version INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime'))
);

CREATE TABLE IF NOT EXISTS weread_article_account (
    account_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    cover_url TEXT NOT NULL DEFAULT '',
    is_enabled INTEGER NOT NULL DEFAULT 0,
    discovered_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_weread_article_account_enabled
    ON weread_article_account(is_enabled, account_id);

-- +goose Down

DROP TABLE IF EXISTS weread_article_account;
DROP TABLE IF EXISTS weread_article_credential;
