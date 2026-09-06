-- +goose Up

DROP INDEX IF EXISTS uq_vpn_subscription_device_user;
CREATE UNIQUE INDEX IF NOT EXISTS uq_vpn_subscription_device_user_profile_active
    ON vpn_subscription_device(user_id, profile_code)
    WHERE status <> 'revoked';

-- +goose Down

DROP INDEX IF EXISTS uq_vpn_subscription_device_user_profile_active;
CREATE UNIQUE INDEX IF NOT EXISTS uq_vpn_subscription_device_user
    ON vpn_subscription_device(user_id);
