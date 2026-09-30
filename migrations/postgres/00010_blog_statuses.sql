-- +goose Up
CREATE TABLE blog_statuses (
    id UUID PRIMARY KEY,
    author_id BIGINT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    images JSONB NOT NULL DEFAULT '[]'::jsonb,
    published BOOLEAN NOT NULL DEFAULT FALSE,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (jsonb_typeof(images) = 'array' AND jsonb_array_length(images) <= 9),
    CHECK (NOT published OR published_at IS NOT NULL)
);
CREATE INDEX blog_statuses_published_time ON blog_statuses(published, published_at DESC, id);
-- +goose Down
DROP TABLE blog_statuses;
