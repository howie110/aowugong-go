-- +goose Up
ALTER TABLE notification_log ADD COLUMN source TEXT;
ALTER TABLE notification_log ADD COLUMN request_id TEXT;
CREATE UNIQUE INDEX idx_notification_log_source_request ON notification_log(source, request_id);

-- +goose Down
DROP INDEX idx_notification_log_source_request;
ALTER TABLE notification_log DROP COLUMN request_id;
ALTER TABLE notification_log DROP COLUMN source;
