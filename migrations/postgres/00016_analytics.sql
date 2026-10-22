-- +goose Up
CREATE TABLE analytics_visits (
 id UUID PRIMARY KEY,
 visitor_id TEXT NOT NULL,
 started_at TIMESTAMPTZ NOT NULL,
 last_at TIMESTAMPTZ NOT NULL,
 entry_path TEXT NOT NULL,
 exit_path TEXT NOT NULL
);
CREATE INDEX analytics_visits_visitor_time ON analytics_visits(visitor_id,last_at DESC);
CREATE INDEX analytics_visits_last_time ON analytics_visits(last_at);
CREATE TABLE analytics_events (
 id UUID PRIMARY KEY,
 visit_id UUID NOT NULL REFERENCES analytics_visits(id) ON DELETE CASCADE,
 visitor_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 path TEXT NOT NULL,
 url TEXT NOT NULL,
 referrer TEXT NOT NULL DEFAULT '',
 channel TEXT NOT NULL DEFAULT '',
 browser TEXT NOT NULL DEFAULT '',
 os TEXT NOT NULL DEFAULT '',
 device TEXT NOT NULL DEFAULT '',
 language TEXT NOT NULL DEFAULT '',
 country TEXT NOT NULL DEFAULT '',
 region TEXT NOT NULL DEFAULT '',
 city TEXT NOT NULL DEFAULT ''
);
CREATE INDEX analytics_events_time ON analytics_events(created_at);
CREATE INDEX analytics_events_visit_time ON analytics_events(visit_id,created_at);
CREATE INDEX analytics_events_path_time ON analytics_events(path,created_at);
-- +goose Down
DROP TABLE analytics_events;
DROP TABLE analytics_visits;
