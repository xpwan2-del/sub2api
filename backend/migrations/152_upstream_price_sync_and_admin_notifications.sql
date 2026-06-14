-- 上游价格同步与变动告警 — 5 张表
--
-- 为上游中转站价格自动拉取、变动检测、双通道告警、建议值一键应用功能建表。
-- 设计文档：docs/UPSTREAM_PRICE_SYNC_DESIGN.md
--
-- 价格体系边界（重要）：
--   - 这 5 张表属于"旁路参考价"，ModelPricingResolver 不读它们。
--   - 唯一进入计费链路的入口是 ApplyService：把参考价/建议倍率写入
--     channel_model_pricing（轴A 单价）或 group.rate_multiplier（轴B 倍率）。
--   - 因此本迁移的价格字段精度严格对齐计费链路：
--       * per-token 价格 → NUMERIC(20,12)   对齐 channel_model_pricing
--       * 图片价格       → NUMERIC(20,8)    对齐 channel_model_pricing.image_output_price
--       * 建议倍率       → DECIMAL(10,4)    对齐 group.rate_multiplier
--       * 百分比派生值   → DOUBLE PRECISION 对齐 ops 监控表
--   - applied_by / user_id 故意不设外键，避免删除用户时卡住审计/已读记录。

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- 1. 上游定价源配置（一个源 = 一个可定时拉取的上游 pricing 接口）
CREATE TABLE IF NOT EXISTS upstream_price_sources (
    id                    BIGSERIAL      PRIMARY KEY,
    name                  VARCHAR(100)   NOT NULL,
    platform              VARCHAR(50)    NOT NULL DEFAULT 'mixed',
    base_url              VARCHAR(500)   NOT NULL,
    pricing_endpoint      VARCHAR(500)   NOT NULL DEFAULT '/api/pricing',
    api_key               VARCHAR(500),                              -- AES-256-GCM 加密后密文，service 层加解密
    parser_type           VARCHAR(30)    NOT NULL DEFAULT 'one_api', -- one_api / new_api / custom
    parser_config         JSONB,                                    -- 解析器配置
    model_alias_map       JSONB,                                    -- 上游模型名 → 本地模型名 映射
    sync_interval_minutes INTEGER       NOT NULL DEFAULT 360,
    alert_threshold_pct   DOUBLE PRECISION NOT NULL DEFAULT 0,      -- 0 = 全部变动都告警
    cooldown_minutes      INTEGER       NOT NULL DEFAULT 60,
    enabled               BOOLEAN       NOT NULL DEFAULT TRUE,
    last_sync_at          TIMESTAMPTZ,
    last_sync_status      VARCHAR(20)   NOT NULL DEFAULT '',        -- success / failed / partial
    last_sync_error       VARCHAR(1000),
    last_hash             VARCHAR(128),                             -- 上次内容哈希，快速判变
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_upstream_price_sources_enabled     ON upstream_price_sources (enabled);
CREATE INDEX IF NOT EXISTS idx_upstream_price_sources_last_sync   ON upstream_price_sources (last_sync_at);

-- 2. 最新参考价快照（每源每模型一条，拉取后整体替换）
CREATE TABLE IF NOT EXISTS upstream_model_prices (
    id                  BIGSERIAL   PRIMARY KEY,
    source_id           BIGINT      NOT NULL REFERENCES upstream_price_sources(id) ON DELETE CASCADE,
    model_name          VARCHAR(255) NOT NULL,
    local_model_name    VARCHAR(255),                            -- 经 alias_map 映射后的本地名（可空）
    input_price         NUMERIC(20,12) NOT NULL,                 -- per-token USD，对齐 channel_model_pricing
    output_price        NUMERIC(20,12) NOT NULL,
    cache_write_price   NUMERIC(20,12),
    cache_read_price    NUMERIC(20,12),
    image_output_price  NUMERIC(20,8),
    per_request_price   NUMERIC(20,12),
    currency            VARCHAR(10) NOT NULL DEFAULT 'USD',
    raw_payload         JSONB,                                  -- 上游原始响应载荷，审计用
    fetched_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_model_prices_source_model
    ON upstream_model_prices (source_id, model_name);

-- 3. 变动记录 + 待应用清单（DiffEngine 检测到的每次变动；status 驱动应用流程）
CREATE TABLE IF NOT EXISTS upstream_price_changes (
    id                      BIGSERIAL   PRIMARY KEY,
    source_id               BIGINT      NOT NULL REFERENCES upstream_price_sources(id) ON DELETE CASCADE,
    model_name              VARCHAR(255) NOT NULL,
    local_model_name        VARCHAR(255),
    change_type             VARCHAR(20) NOT NULL,                -- added / removed / price_change
    prev_input_price        NUMERIC(20,12),                      -- 新增模型时为空
    prev_output_price       NUMERIC(20,12),
    curr_input_price        NUMERIC(20,12) NOT NULL,
    curr_output_price       NUMERIC(20,12) NOT NULL,
    input_delta_pct         DOUBLE PRECISION NOT NULL DEFAULT 0,
    output_delta_pct        DOUBLE PRECISION NOT NULL DEFAULT 0,
    detected_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notified                BOOLEAN     NOT NULL DEFAULT FALSE,
    status                  VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending / applied / dismissed
    suggested_input_price   NUMERIC(20,12) NOT NULL DEFAULT 0,
    suggested_output_price  NUMERIC(20,12) NOT NULL DEFAULT 0,
    suggested_multiplier    DECIMAL(10,4),                       -- 锁死售价模式算出，对齐 group.rate_multiplier
    applied_at              TIMESTAMPTZ,
    applied_by              BIGINT,                              -- 操作人 admin user id（不设外键）
    applied_target          VARCHAR(30),                         -- account / group / model_config
    applied_target_id       BIGINT      NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_upstream_price_changes_status        ON upstream_price_changes (status);
CREATE INDEX IF NOT EXISTS idx_upstream_price_changes_source_time   ON upstream_price_changes (source_id, detected_at);

-- 4. admin 站内通知（独立于面向终端用户的 announcement，避免内部成本信息泄露）
CREATE TABLE IF NOT EXISTS admin_notifications (
    id           BIGSERIAL   PRIMARY KEY,
    type         VARCHAR(40) NOT NULL DEFAULT 'system',          -- system / price_change / ops_alert
    title        VARCHAR(200) NOT NULL,
    content      TEXT        NOT NULL,                           -- 支持 Markdown
    severity     VARCHAR(20) NOT NULL DEFAULT 'info',            -- info / warning / critical
    target_link  VARCHAR(500),
    related_ids  JSONB,                                          -- 关联实体 ID 数组（如 change_ids）
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_notifications_severity ON admin_notifications (severity);
CREATE INDEX IF NOT EXISTS idx_admin_notifications_created  ON admin_notifications (created_at);

-- 5. admin 通知已读记录（多 admin 独立已读，对齐 announcement_read）
CREATE TABLE IF NOT EXISTS admin_notification_reads (
    id              BIGSERIAL   PRIMARY KEY,
    notification_id BIGINT      NOT NULL REFERENCES admin_notifications(id) ON DELETE CASCADE,
    user_id         BIGINT      NOT NULL,                         -- admin user id（不设外键）
    read_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_notification_reads_notification ON admin_notification_reads (notification_id);
CREATE INDEX IF NOT EXISTS idx_admin_notification_reads_user        ON admin_notification_reads (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_admin_notification_reads_notif_user
    ON admin_notification_reads (notification_id, user_id);

COMMENT ON TABLE upstream_price_sources    IS '上游定价源配置：一个可定时拉取的上游 pricing 接口';
COMMENT ON TABLE upstream_model_prices     IS '上游参考价快照：旁路表，不进计费链路，唯一入口是 ApplyService';
COMMENT ON TABLE upstream_price_changes    IS '上游价格变动记录 + 待应用清单：status 驱动 pending→applied/dismissed';
COMMENT ON TABLE admin_notifications       IS '管理员站内通知：价格变动告警等，独立于面向用户的 announcement';
COMMENT ON TABLE admin_notification_reads  IS '管理员通知已读：多 admin 独立已读时间';

COMMENT ON COLUMN upstream_model_prices.input_price    IS '每 token 输入价格（USD），精度对齐 channel_model_pricing';
COMMENT ON COLUMN upstream_model_prices.output_price   IS '每 token 输出价格（USD），精度对齐 channel_model_pricing';
COMMENT ON COLUMN upstream_price_changes.curr_input_price  IS '当前输入价格（removed 变动时由应用层语义决定）';
COMMENT ON COLUMN upstream_price_changes.suggested_multiplier IS '锁死售价建议倍率，写入 group.rate_multiplier（DECIMAL(10,4)）';
COMMENT ON COLUMN upstream_price_sources.api_key       IS 'AES-256-GCM 加密密文，加解密在 service 层（ent Sensitive 仅脱敏）';
