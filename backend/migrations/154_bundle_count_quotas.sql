-- 154_bundle_count_quotas.sql
-- 套餐按次计费：为额度表与用量表增加次数维度（对称双轨，0=不限）。

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE bundle_plan_group_quotas
    ADD COLUMN IF NOT EXISTS daily_limit_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_limit_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_limit_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE bundle_subscription_usages
    ADD COLUMN IF NOT EXISTS daily_usage_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_usage_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_usage_count INTEGER NOT NULL DEFAULT 0;
