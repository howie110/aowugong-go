-- +goose Up

CREATE TABLE IF NOT EXISTS vpn_subscription_device (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    profile_code TEXT NOT NULL,
    token_version INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'draft',
    published_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (DATETIME('now', 'localtime')),
    updated_at TEXT NOT NULL DEFAULT (DATETIME('now', 'localtime')),
    CONSTRAINT uq_vpn_subscription_device_name UNIQUE(name)
);
CREATE INDEX IF NOT EXISTS idx_vpn_subscription_device_status
    ON vpn_subscription_device(status, id);

-- +goose Down

DROP TABLE IF EXISTS vpn_subscription_device;
