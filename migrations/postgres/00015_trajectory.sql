-- +goose Up
CREATE TABLE trajectory_devices (
 id UUID PRIMARY KEY,
 owner_id BIGINT NOT NULL REFERENCES aowugong_fastapi_users(id),
 name TEXT NOT NULL,
 token_hash TEXT NOT NULL UNIQUE,
 created_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ,
 last_received_at TIMESTAMPTZ
);
CREATE INDEX trajectory_devices_owner ON trajectory_devices(owner_id);
CREATE TABLE trajectory_points (
 id BIGSERIAL PRIMARY KEY,
 device_id UUID NOT NULL REFERENCES trajectory_devices(id),
 recorded_at TIMESTAMPTZ NOT NULL,
 received_at TIMESTAMPTZ NOT NULL,
 lat DOUBLE PRECISION NOT NULL CHECK(lat BETWEEN -90 AND 90),
 lon DOUBLE PRECISION NOT NULL CHECK(lon BETWEEN -180 AND 180),
 accuracy DOUBLE PRECISION,
 altitude DOUBLE PRECISION,
 speed DOUBLE PRECISION,
 bearing DOUBLE PRECISION,
 provider TEXT NOT NULL DEFAULT '',
 raw_payload JSONB NOT NULL,
 UNIQUE(device_id,recorded_at,lat,lon)
);
CREATE INDEX trajectory_points_device_time ON trajectory_points(device_id,recorded_at DESC,id DESC);
-- +goose Down
DROP TABLE trajectory_points;
DROP TABLE trajectory_devices;
