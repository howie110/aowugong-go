-- +goose Up

CREATE TABLE IF NOT EXISTS investment_article_model_setting (
    id SMALLINT PRIMARY KEY CHECK(id = 1),
    model_id TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS'))
);

-- +goose Down

DROP TABLE IF EXISTS investment_article_model_setting;
