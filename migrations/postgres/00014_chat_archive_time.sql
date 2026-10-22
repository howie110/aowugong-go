-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION chat_archive_source_time(value text) RETURNS timestamptz
LANGUAGE plpgsql IMMUTABLE SET timezone='UTC' AS $$
BEGIN
 IF value IS NULL OR length(value)>1000 OR btrim(value) !~ '^\d{4}-\d{2}-\d{2}([T ]([01]\d|2[0-3]):[0-5]\d:[0-5]\d(\.\d+)?(Z|[+-]\d{2}:\d{2})?)?$' THEN RETURN NULL; END IF;
 IF position('T' in value)>0 AND btrim(value) !~ '(Z|[+-]\d{2}:\d{2})$' THEN RETURN NULL; END IF;
 RETURN btrim(value)::timestamptz;
EXCEPTION WHEN invalid_datetime_format OR datetime_field_overflow THEN RETURN NULL;
END;
$$;
-- +goose StatementEnd
ALTER TABLE chat_archive_message_versions ADD COLUMN source_at timestamptz GENERATED ALWAYS AS (chat_archive_source_time(source_time)) STORED;
CREATE INDEX chat_archive_messages_source_at ON chat_archive_message_versions(source_at DESC NULLS LAST,id);
-- +goose Down
ALTER TABLE chat_archive_message_versions DROP COLUMN source_at;
DROP FUNCTION chat_archive_source_time(text);
