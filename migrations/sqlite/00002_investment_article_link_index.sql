-- +goose Up

CREATE INDEX IF NOT EXISTS idx_investment_article_link
    ON investment_article(link);

-- +goose Down

DROP INDEX IF EXISTS idx_investment_article_link;
