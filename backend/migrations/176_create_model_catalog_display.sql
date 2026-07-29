SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS model_catalog_displays (
    id              BIGSERIAL    PRIMARY KEY,
    platform        VARCHAR(64)  NOT NULL,
    model_name      VARCHAR(128) NOT NULL,
    pinned          BOOLEAN      NOT NULL DEFAULT FALSE,
    sort_weight     INTEGER      NOT NULL DEFAULT 0,
    custom_tags     JSONB        NOT NULL DEFAULT '[]',
    featured_until  TIMESTAMPTZ,
    hidden          BOOLEAN      NOT NULL DEFAULT FALSE,
    first_seen_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (platform, model_name)
);
COMMENT ON TABLE model_catalog_displays IS '模型广场展示配置（置顶/标签/隐藏），按平台+模型名';
