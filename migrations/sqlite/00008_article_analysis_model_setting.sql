-- +goose Up

CREATE TABLE IF NOT EXISTS investment_article_model_setting (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    model_id TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now', '+8 hours'))
);

-- +goose Down

DROP TABLE IF EXISTS investment_article_model_setting;
