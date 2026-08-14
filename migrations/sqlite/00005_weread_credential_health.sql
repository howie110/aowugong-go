-- +goose Up

ALTER TABLE weread_article_credential ADD COLUMN bound_at TEXT;
ALTER TABLE weread_article_credential ADD COLUMN last_checked_at TEXT;
ALTER TABLE weread_article_credential ADD COLUMN last_valid_at TEXT;
ALTER TABLE weread_article_credential ADD COLUMN invalid_at TEXT;
ALTER TABLE weread_article_credential ADD COLUMN last_status TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE weread_article_credential ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE weread_article_credential ADD COLUMN check_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE weread_article_credential ADD COLUMN refresh_count INTEGER NOT NULL DEFAULT 0;

UPDATE weread_article_credential
SET bound_at = COALESCE(bound_at, updated_at),
    last_valid_at = COALESCE(last_valid_at, updated_at),
    last_status = CASE WHEN last_status = 'unknown' THEN 'valid' ELSE last_status END
WHERE id = 1;

-- +goose Down

ALTER TABLE weread_article_credential DROP COLUMN refresh_count;
ALTER TABLE weread_article_credential DROP COLUMN check_count;
ALTER TABLE weread_article_credential DROP COLUMN last_error;
ALTER TABLE weread_article_credential DROP COLUMN last_status;
ALTER TABLE weread_article_credential DROP COLUMN invalid_at;
ALTER TABLE weread_article_credential DROP COLUMN last_valid_at;
ALTER TABLE weread_article_credential DROP COLUMN last_checked_at;
ALTER TABLE weread_article_credential DROP COLUMN bound_at;
