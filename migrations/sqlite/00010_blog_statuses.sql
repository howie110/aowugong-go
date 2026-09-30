-- +goose Up
CREATE TABLE blog_statuses (
    id TEXT PRIMARY KEY,
    author_id INTEGER NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    images TEXT NOT NULL DEFAULT '[]',
    published BOOLEAN NOT NULL DEFAULT FALSE,
    published_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CHECK (NOT published OR published_at IS NOT NULL)
);
CREATE INDEX blog_statuses_published_time ON blog_statuses(published, published_at DESC, id);
-- +goose Down
DROP TABLE blog_statuses;
