-- 156_bundle_image_video_split_quota.sql
-- 套餐次数限额拆分图片/视频:现有 count 复用为图片(RENAME 保留旧值),新增视频。
-- 迁移 SHA256 锁定只跑一次,RENAME 非幂等可接受。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- bundle_plan_group_quotas (限额配置)
ALTER TABLE bundle_plan_group_quotas RENAME COLUMN daily_limit_count   TO daily_image_limit_count;
ALTER TABLE bundle_plan_group_quotas RENAME COLUMN weekly_limit_count  TO weekly_image_limit_count;
ALTER TABLE bundle_plan_group_quotas RENAME COLUMN monthly_limit_count TO monthly_image_limit_count;
ALTER TABLE bundle_plan_group_quotas
    ADD COLUMN IF NOT EXISTS daily_video_limit_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_video_limit_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_video_limit_count INTEGER NOT NULL DEFAULT 0;

-- bundle_subscription_usages (用量跟踪)
ALTER TABLE bundle_subscription_usages RENAME COLUMN daily_usage_count   TO daily_image_usage_count;
ALTER TABLE bundle_subscription_usages RENAME COLUMN weekly_usage_count  TO weekly_image_usage_count;
ALTER TABLE bundle_subscription_usages RENAME COLUMN monthly_usage_count TO monthly_image_usage_count;
ALTER TABLE bundle_subscription_usages
    ADD COLUMN IF NOT EXISTS daily_video_usage_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_video_usage_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_video_usage_count INTEGER NOT NULL DEFAULT 0;

-- user_subscriptions (限额快照)
ALTER TABLE user_subscriptions RENAME COLUMN daily_limit_count   TO daily_image_limit_count;
ALTER TABLE user_subscriptions RENAME COLUMN weekly_limit_count  TO weekly_image_limit_count;
ALTER TABLE user_subscriptions RENAME COLUMN monthly_limit_count TO monthly_image_limit_count;
ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS daily_video_limit_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_video_limit_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_video_limit_count INTEGER NOT NULL DEFAULT 0;

-- usage_logs (明细)
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS video_count INTEGER NOT NULL DEFAULT 0;
