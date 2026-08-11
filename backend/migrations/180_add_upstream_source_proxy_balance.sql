SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE upstream_source_configs
    ADD COLUMN IF NOT EXISTS proxy_id BIGINT REFERENCES proxies(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS last_balance_quota BIGINT,
    ADD COLUMN IF NOT EXISTS last_used_quota BIGINT,
    ADD COLUMN IF NOT EXISTS last_balance_usd NUMERIC(20,6),
    ADD COLUMN IF NOT EXISTS last_balance_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_balance_checked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_balance_error TEXT;

CREATE INDEX IF NOT EXISTS idx_upstream_source_configs_proxy_id
    ON upstream_source_configs (proxy_id);
