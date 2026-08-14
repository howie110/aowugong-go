-- +goose Up

ALTER TABLE weread_article_account
    ADD COLUMN fetch_interval_minutes INTEGER NOT NULL DEFAULT 720;

ALTER TABLE weread_article_account
    ADD COLUMN fetch_limit INTEGER NOT NULL DEFAULT 20;

ALTER TABLE weread_article_account
    ADD COLUMN last_checked_at TEXT;

UPDATE weread_article_account
SET fetch_interval_minutes = 720
WHERE fetch_interval_minutes < 1;

UPDATE weread_article_account
SET fetch_limit = 20
WHERE fetch_limit < 1;

-- +goose Down

CREATE TABLE weread_article_account_new (
    account_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    cover_url TEXT NOT NULL DEFAULT '',
    is_enabled INTEGER NOT NULL DEFAULT 0,
    discovered_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime'))
);

INSERT INTO weread_article_account_new(account_id, title, cover_url, is_enabled, discovered_at, updated_at)
SELECT account_id, title, cover_url, is_enabled, discovered_at, updated_at FROM weread_article_account;

DROP TABLE weread_article_account;
ALTER TABLE weread_article_account_new RENAME TO weread_article_account;

CREATE INDEX IF NOT EXISTS idx_weread_article_account_enabled
    ON weread_article_account(is_enabled, account_id);
