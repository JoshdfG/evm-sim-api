-- 002_create_simulations.sql
-- Stores every simulation run for billing, replay, and analytics.

CREATE TABLE IF NOT EXISTS simulations (
    id            UUID        PRIMARY KEY,
    requested_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Transaction metadata — indexed for fast customer queries.
    chain_id      BIGINT      NOT NULL,
    from_address  TEXT        NOT NULL,
    to_address    TEXT        NOT NULL,
    block_number  BIGINT,

    -- Outcome summary — indexed for dashboard filtering.
    success       BOOLEAN     NOT NULL,
    revert_reason TEXT,
    gas_used      BIGINT,
    risk_level    TEXT        NOT NULL DEFAULT 'none', -- 'none' | 'warning' | 'critical'

    -- API key attribution for per-customer billing.
    api_key       TEXT        REFERENCES api_keys(key) ON DELETE SET NULL,

    -- Full payloads stored as JSONB for flexible ad-hoc querying.
    request_json  JSONB       NOT NULL,
    result_json   JSONB       NOT NULL
);

-- Query patterns: by address, chain, key, time.
CREATE INDEX IF NOT EXISTS idx_sim_from_address  ON simulations (from_address);
CREATE INDEX IF NOT EXISTS idx_sim_to_address    ON simulations (to_address);
CREATE INDEX IF NOT EXISTS idx_sim_chain_id      ON simulations (chain_id);
CREATE INDEX IF NOT EXISTS idx_sim_api_key       ON simulations (api_key);
CREATE INDEX IF NOT EXISTS idx_sim_requested_at  ON simulations (requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_sim_risk_level    ON simulations (risk_level) WHERE risk_level != 'none';

-- GIN index enables queries inside the JSONB blobs:
--   SELECT * FROM simulations WHERE result_json @> '{"success": false}';
CREATE INDEX IF NOT EXISTS idx_sim_result_gin ON simulations USING GIN (result_json);
