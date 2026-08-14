-- +goose Up

ALTER TABLE weread_article_credential
    ADD COLUMN IF NOT EXISTS bound_at TEXT,
    ADD COLUMN IF NOT EXISTS last_checked_at TEXT,
    ADD COLUMN IF NOT EXISTS last_valid_at TEXT,
    ADD COLUMN IF NOT EXISTS invalid_at TEXT,
    ADD COLUMN IF NOT EXISTS last_status TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS check_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS refresh_count BIGINT NOT NULL DEFAULT 0;

UPDATE weread_article_credential
SET bound_at = COALESCE(bound_at, updated_at),
    last_valid_at = COALESCE(last_valid_at, updated_at),
    last_status = CASE WHEN last_status = 'unknown' THEN 'valid' ELSE last_status END
WHERE id = 1;

-- +goose Down

ALTER TABLE weread_article_credential
    DROP COLUMN IF EXISTS refresh_count,
    DROP COLUMN IF EXISTS check_count,
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS last_status,
    DROP COLUMN IF EXISTS invalid_at,
    DROP COLUMN IF EXISTS last_valid_at,
    DROP COLUMN IF EXISTS last_checked_at,
    DROP COLUMN IF EXISTS bound_at;
