-- Add per-channel recharge multiplier for unified balance/rate observability.
ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS recharge_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1;
