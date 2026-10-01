-- +goose Up
CREATE TABLE blog_articles (
    slug TEXT PRIMARY KEY,
    markdown TEXT NOT NULL,
    title TEXT NOT NULL,
    date TEXT NOT NULL,
    tags TEXT NOT NULL,
    html TEXT NOT NULL,
    description TEXT NOT NULL,
    toc TEXT NOT NULL
);
CREATE INDEX blog_articles_date ON blog_articles(date DESC, slug);
CREATE TABLE blog_article_assets (
    path TEXT PRIMARY KEY,
    content BLOB NOT NULL,
    content_type TEXT NOT NULL
);
CREATE TABLE blog_article_publication (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    sequence BIGINT NOT NULL
);
INSERT INTO blog_article_publication (id, sequence) VALUES (1, 0);
-- +goose Down
DROP TABLE blog_article_assets;
DROP TABLE blog_articles;
DROP TABLE blog_article_publication;
