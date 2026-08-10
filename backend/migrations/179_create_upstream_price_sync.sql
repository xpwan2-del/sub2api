SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS upstream_source_configs (
    id                        BIGSERIAL     PRIMARY KEY,
    name                      VARCHAR(128)  NOT NULL,
    base_url                  TEXT          NOT NULL,
    api_key_encrypted         TEXT          NOT NULL DEFAULT '',
    dashboard_token_encrypted TEXT          NOT NULL DEFAULT '',
    target_channel_id         BIGINT        NOT NULL REFERENCES channels(id) ON DELETE RESTRICT,
    enabled                   BOOLEAN       NOT NULL DEFAULT TRUE,
    base_price_per_1k         NUMERIC(10,6) NOT NULL DEFAULT 0.002,
    pricing_source            VARCHAR(20)   NOT NULL DEFAULT 'auto',
    sync_model_price          BOOLEAN       NOT NULL DEFAULT TRUE,
    sync_group_ratio          BOOLEAN       NOT NULL DEFAULT FALSE,
    group_mapping             JSONB         NOT NULL DEFAULT '{}'::jsonb,
    balance_threshold_usd     NUMERIC(12,4),
    last_sync_at              TIMESTAMPTZ,
    last_pricing_version      VARCHAR(128),
    last_error                TEXT,
    created_at                TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ   NOT NULL DEFAULT now()
);
COMMENT ON TABLE upstream_source_configs IS '上游 new-api 定价同步源配置';
CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_source_configs_name ON upstream_source_configs (name);

CREATE TABLE IF NOT EXISTS upstream_price_change_requests (
    id                       BIGSERIAL    PRIMARY KEY,
    source_config_id         BIGINT       NOT NULL REFERENCES upstream_source_configs(id) ON DELETE CASCADE,
    trigger_type             VARCHAR(20)  NOT NULL,
    status                   VARCHAR(20)  NOT NULL DEFAULT 'open',
    upstream_pricing_version VARCHAR(128),
    summary                  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_by               BIGINT,
    created_at               TIMESTAMPTZ  NOT NULL DEFAULT now(),
    closed_at                TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_upcr_source_status ON upstream_price_change_requests (source_config_id, status);
CREATE INDEX IF NOT EXISTS idx_upcr_created ON upstream_price_change_requests (created_at);

CREATE TABLE IF NOT EXISTS upstream_price_change_items (
    id                 BIGSERIAL   PRIMARY KEY,
    request_id         BIGINT      NOT NULL REFERENCES upstream_price_change_requests(id) ON DELETE CASCADE,
    kind               VARCHAR(20) NOT NULL,
    platform           VARCHAR(32),
    model_name         VARCHAR(128),
    target_channel_id  BIGINT,
    upstream_raw       JSONB,
    upstream_converted JSONB,
    local_current      JSONB,
    apply_value        JSONB,
    status             VARCHAR(20) NOT NULL DEFAULT 'pending',
    reviewer_id        BIGINT,
    review_note        TEXT,
    reviewed_at        TIMESTAMPTZ,
    applied_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_upci_request_status ON upstream_price_change_items (request_id, status);
