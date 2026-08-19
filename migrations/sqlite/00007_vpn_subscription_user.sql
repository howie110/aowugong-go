-- +goose Up

ALTER TABLE vpn_subscription_device ADD COLUMN user_id INTEGER
    REFERENCES aowugong_fastapi_users(id) ON DELETE CASCADE;

DELETE FROM vpn_subscription_device
WHERE user_id IS NULL
  AND id <> (SELECT MAX(id) FROM vpn_subscription_device WHERE user_id IS NULL);

UPDATE vpn_subscription_device
SET user_id = (
        SELECT id
        FROM aowugong_fastapi_users
        WHERE is_active = 1
        ORDER BY is_superuser DESC, id
        LIMIT 1
    ),
    name = COALESCE((
        SELECT username
        FROM aowugong_fastapi_users
        WHERE is_active = 1
        ORDER BY is_superuser DESC, id
        LIMIT 1
    ), name),
    updated_at = DATETIME('now', 'localtime')
WHERE user_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_vpn_subscription_device_user
    ON vpn_subscription_device(user_id);

CREATE INDEX IF NOT EXISTS idx_vpn_subscription_device_user_status
    ON vpn_subscription_device(user_id, status);

-- +goose Down

DROP INDEX IF EXISTS idx_vpn_subscription_device_user_status;
DROP INDEX IF EXISTS uq_vpn_subscription_device_user;
ALTER TABLE vpn_subscription_device DROP COLUMN user_id;
