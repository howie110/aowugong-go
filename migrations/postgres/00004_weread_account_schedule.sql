-- +goose Up

ALTER TABLE weread_article_account
    ADD COLUMN IF NOT EXISTS fetch_interval_minutes INTEGER NOT NULL DEFAULT 720,
    ADD COLUMN IF NOT EXISTS fetch_limit INTEGER NOT NULL DEFAULT 20,
    ADD COLUMN IF NOT EXISTS last_checked_at TEXT;

UPDATE weread_article_account
SET fetch_interval_minutes = 720
WHERE fetch_interval_minutes < 1;

UPDATE weread_article_account
SET fetch_limit = 20
WHERE fetch_limit < 1;

-- +goose Down

ALTER TABLE weread_article_account
    DROP COLUMN IF EXISTS last_checked_at,
    DROP COLUMN IF EXISTS fetch_limit,
    DROP COLUMN IF EXISTS fetch_interval_minutes;
