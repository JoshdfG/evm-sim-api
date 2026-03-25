-- 001_create_api_keys.sql
-- Run this migration first — simulations table references api_keys.

CREATE TABLE IF NOT EXISTS api_keys (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    key        TEXT        NOT NULL UNIQUE,
    owner_id   TEXT        NOT NULL,
    plan       TEXT        NOT NULL DEFAULT 'free',  -- 'free' | 'pro' | 'enterprise'
    label      TEXT,                                  -- human-readable nickname
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ                            -- NULL = active key
);

CREATE INDEX IF NOT EXISTS idx_api_keys_key      ON api_keys (key);
CREATE INDEX IF NOT EXISTS idx_api_keys_owner_id ON api_keys (owner_id);
