-- +goose Up
CREATE TABLE chat_archive_conversations (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    account_scope TEXT NOT NULL,
    source_id TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source, account_scope, source_id)
);
CREATE TABLE chat_archive_raw_batches (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES chat_archive_conversations(id),
    source_key TEXT NOT NULL,
    device_id TEXT NOT NULL,
    source_path TEXT NOT NULL,
    start_offset BIGINT NOT NULL CHECK (start_offset >= 0),
    raw BYTEA NOT NULL,
    sha256 TEXT NOT NULL,
    parser_version TEXT NOT NULL,
    unknown_events INTEGER NOT NULL DEFAULT 0,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE chat_archive_message_versions (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES chat_archive_conversations(id),
    message_key TEXT NOT NULL,
    version_hash TEXT NOT NULL,
    batch_id BIGINT NOT NULL REFERENCES chat_archive_raw_batches(id),
    position BIGINT NOT NULL,
    parent_key TEXT,
    identity_quality TEXT NOT NULL,
    role TEXT NOT NULL,
    body TEXT NOT NULL,
    blocks JSONB NOT NULL,
    attachments JSONB NOT NULL,
    completeness TEXT NOT NULL,
    source_time TEXT,
    UNIQUE(batch_id, position)
);
CREATE INDEX chat_archive_messages_identity ON chat_archive_message_versions(conversation_id, message_key, id DESC);
CREATE INDEX chat_archive_batches_conversation ON chat_archive_raw_batches(conversation_id, id);
CREATE INDEX chat_archive_conversations_updated ON chat_archive_conversations(updated_at DESC, id DESC);
CREATE TABLE chat_archive_sync_states (
    source_key TEXT PRIMARY KEY,
    state JSONB,
    last_success_at TIMESTAMPTZ,
    last_error TEXT,
    last_failure_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE chat_archive_sync_states;
DROP TABLE chat_archive_message_versions;
DROP TABLE chat_archive_raw_batches;
DROP TABLE chat_archive_conversations;
