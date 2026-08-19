-- +goose Up

ALTER TABLE vpn_subscription_device
    ADD COLUMN IF NOT EXISTS user_id BIGINT
    REFERENCES aowugong_fastapi_users(id) ON DELETE CASCADE;

DELETE FROM vpn_subscription_device
WHERE user_id IS NULL
  AND id <> (SELECT MAX(id) FROM vpn_subscription_device WHERE user_id IS NULL);

UPDATE vpn_subscription_device AS subscription
SET user_id = selected_user.id,
    name = selected_user.username,
    updated_at = TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')
FROM (
    SELECT app_user.id, app_user.username
    FROM aowugong_fastapi_users AS app_user
    LEFT JOIN aowugong_user_roles AS user_role ON user_role.user_id = app_user.id
    LEFT JOIN aowugong_roles AS role ON role.id = user_role.role_id AND role.code = 'admin'
    WHERE app_user.is_active = 1
    ORDER BY app_user.is_superuser DESC, (role.id IS NOT NULL) DESC, app_user.id
    LIMIT 1
) AS selected_user
WHERE subscription.user_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_vpn_subscription_device_user
    ON vpn_subscription_device(user_id);

CREATE INDEX IF NOT EXISTS idx_vpn_subscription_device_user_status
    ON vpn_subscription_device(user_id, status);

-- +goose Down

DROP INDEX IF EXISTS idx_vpn_subscription_device_user_status;
DROP INDEX IF EXISTS uq_vpn_subscription_device_user;
ALTER TABLE vpn_subscription_device DROP COLUMN IF EXISTS user_id;
