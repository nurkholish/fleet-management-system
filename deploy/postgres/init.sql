CREATE TABLE IF NOT EXISTS vehicle_locations (
    id         BIGSERIAL PRIMARY KEY,
    vehicle_id VARCHAR(32) NOT NULL,
    latitude   DOUBLE PRECISION NOT NULL,
    longitude  DOUBLE PRECISION NOT NULL,
    timestamp  BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Idempotency: dedupe at-least-once delivery
CREATE UNIQUE INDEX IF NOT EXISTS idx_vehicle_ts_unique
    ON vehicle_locations (vehicle_id, timestamp);

-- Fast latest lookup
CREATE INDEX IF NOT EXISTS idx_vehicle_ts_desc
    ON vehicle_locations (vehicle_id, timestamp DESC);