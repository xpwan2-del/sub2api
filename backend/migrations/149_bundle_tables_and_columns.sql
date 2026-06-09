-- 149_bundle_tables_and_columns.sql
-- 套餐捆绑销售系统：创建套餐相关数据表，并在现有表中添加套餐关联字段。
-- 此迁移是套餐功能的数据库基础，建立 4 张新表 + 2 张表的字段扩展。

-- ============================================================
-- 1. bundle_plans: 套餐计划定义
-- ============================================================
CREATE TABLE IF NOT EXISTS bundle_plans (
    id                BIGSERIAL    PRIMARY KEY,
    name              TEXT         NOT NULL,
    description       TEXT         NOT NULL DEFAULT '',
    tier              TEXT         NOT NULL,                        -- basic/flagship/enterprise
    price             DOUBLE PRECISION NOT NULL DEFAULT 0,
    original_price    DOUBLE PRECISION NOT NULL DEFAULT 0,
    currency          TEXT         NOT NULL DEFAULT 'USD',
    validity_days     INTEGER      NOT NULL DEFAULT 30,
    concurrency_limit INTEGER      NOT NULL DEFAULT 0,             -- 0=不限
    rpm_limit         INTEGER      NOT NULL DEFAULT 0,             -- 0=不限
    features          JSONB        DEFAULT NULL,
    for_sale          BOOLEAN      NOT NULL DEFAULT TRUE,
    sort_order        INTEGER      NOT NULL DEFAULT 0,
    status            TEXT         NOT NULL DEFAULT 'active',      -- active/disabled
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 索引：按状态+在售标记查询（管理后台列表）
CREATE INDEX IF NOT EXISTS bundleplan_status_for_sale ON bundle_plans (status, for_sale);
-- 索引：按层级查询（筛选 starter/pro/enterprise）
CREATE INDEX IF NOT EXISTS bundleplan_tier            ON bundle_plans (tier);

-- ============================================================
-- 2. bundle_plan_group_quotas: 套餐计划 → Group 额度映射
-- ============================================================
CREATE TABLE IF NOT EXISTS bundle_plan_group_quotas (
    id              BIGSERIAL      PRIMARY KEY,
    plan_id         BIGINT         NOT NULL REFERENCES bundle_plans(id) ON DELETE CASCADE,
    group_id        BIGINT         NOT NULL,
    quota_scope     TEXT           NOT NULL DEFAULT 'platform',      -- platform/model
    model_pattern   TEXT           NOT NULL DEFAULT '',              -- 仅 model 级别生效，glob 模式
    daily_limit_usd   DOUBLE PRECISION NOT NULL DEFAULT 0,
    weekly_limit_usd  DOUBLE PRECISION NOT NULL DEFAULT 0,
    monthly_limit_usd DOUBLE PRECISION NOT NULL DEFAULT 0
);

-- 索引：按计划ID+渠道组ID联合查询（加载计划的额度配置）
CREATE INDEX IF NOT EXISTS bundleplangroupquota_plan_id_group_id ON bundle_plan_group_quotas (plan_id, group_id);

-- ============================================================
-- 3. bundle_subscriptions: 用户购买的套餐实例
-- ============================================================
CREATE TABLE IF NOT EXISTS bundle_subscriptions (
    id                BIGSERIAL    PRIMARY KEY,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ  DEFAULT NULL,
    user_id           BIGINT       NOT NULL,
    plan_id           BIGINT       NOT NULL REFERENCES bundle_plans(id) ON DELETE CASCADE,
    status            TEXT         NOT NULL DEFAULT 'active',      -- active/expired/revoked
    starts_at         TIMESTAMPTZ  NOT NULL,
    expires_at        TIMESTAMPTZ  NOT NULL,
    concurrency_limit INTEGER      NOT NULL DEFAULT 0,             -- 快照：并发上限
    rpm_limit         INTEGER      NOT NULL DEFAULT 0,             -- 快照：RPM上限
    source            TEXT         NOT NULL DEFAULT 'purchase'     -- purchase/redeem/admin_assign
);

-- 索引：按用户ID+状态+到期时间查询（获取用户活跃订阅）
CREATE INDEX IF NOT EXISTS bundlesubscription_user_id_status_expires_at ON bundle_subscriptions (user_id, status, expires_at);
-- 索引：按计划ID查询（统计计划订阅数）
CREATE INDEX IF NOT EXISTS bundlesubscription_plan_id                   ON bundle_subscriptions (plan_id);

-- ============================================================
-- 4. bundle_subscription_usages: 套餐实例用量跟踪
-- ============================================================
CREATE TABLE IF NOT EXISTS bundle_subscription_usages (
    id                      BIGSERIAL      PRIMARY KEY,
    bundle_subscription_id  BIGINT         NOT NULL REFERENCES bundle_subscriptions(id) ON DELETE CASCADE,
    group_id                BIGINT         NOT NULL,
    model_pattern           TEXT           NOT NULL DEFAULT '',           -- 空=平台级，有值=模型级
    daily_usage_usd         DOUBLE PRECISION NOT NULL DEFAULT 0,
    daily_window_start      TIMESTAMPTZ    NOT NULL DEFAULT '2000-01-01T00:00:00Z',
    weekly_usage_usd        DOUBLE PRECISION NOT NULL DEFAULT 0,
    weekly_window_start     TIMESTAMPTZ    NOT NULL DEFAULT '2000-01-01T00:00:00Z',
    monthly_usage_usd       DOUBLE PRECISION NOT NULL DEFAULT 0,
    monthly_window_start    TIMESTAMPTZ    NOT NULL DEFAULT '2000-01-01T00:00:00Z'
);

-- 索引：按订阅ID+渠道组ID联合查询（加载订阅的用量数据）
CREATE INDEX IF NOT EXISTS bundlesubscriptionusage_bundle_subscription_id_group_id ON bundle_subscription_usages (bundle_subscription_id, group_id);

-- ============================================================
-- 5. user_subscriptions: 添加 bundle 关联列
-- ============================================================
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS bundle_subscription_id BIGINT DEFAULT NULL;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS daily_limit_usd        DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS weekly_limit_usd       DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS monthly_limit_usd      DOUBLE PRECISION NOT NULL DEFAULT 0;

-- 索引：按套餐订阅ID查询桥接的 UserSubscription（同步状态时使用）
CREATE INDEX IF NOT EXISTS usersubscription_bundle_subscription_id ON user_subscriptions (bundle_subscription_id);

-- ============================================================
-- 6. api_keys: 添加 bundle 关联列
-- ============================================================
-- api_keys 添加套餐订阅关联列，用于网关中间件识别套餐 Key 并进行路由解析
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS bundle_subscription_id BIGINT DEFAULT NULL;
