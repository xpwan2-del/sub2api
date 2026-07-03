-- 159_bundle_unique_constraints.sql
-- 套餐数据完整性：补唯一约束与索引，为并发安全提供 DB 层兜底。
--
-- 背景：ActivateBundle 已事务化，但 DB 层缺唯一约束兜底——历史/并发仍可能产生
-- 同一用户的多个 active 订阅、或同一 (订阅,组,pattern) 的重复 usage 行。
-- 重复 usage 行会让 GetBySubscriptionAndGroup 的 .Only() 抛 "more than one row"，
-- 导致用量扣减停摆。本迁移先去重存量，再加唯一约束与批量过期扫描索引。
--
-- 注意：去重会改动存量数据，生产应用前请先 SELECT 核查重复量。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- 1. 去重 bundle_subscription_usages：按 (订阅,组,pattern) 保留最新 id，删多余行。
DELETE FROM bundle_subscription_usages
WHERE id NOT IN (
    SELECT MAX(id) FROM bundle_subscription_usages
    GROUP BY bundle_subscription_id, group_id, model_pattern
);

-- 2. 去重 bundle_subscriptions：同一 user 的多个未删除 active 订阅，保留最新，其余降级 revoked。
UPDATE bundle_subscriptions
SET status = 'revoked', updated_at = NOW()
WHERE status = 'active'
  AND deleted_at IS NULL
  AND id NOT IN (
      SELECT MAX(id) FROM bundle_subscriptions
      WHERE status = 'active' AND deleted_at IS NULL
      GROUP BY user_id
  );

-- 3. partial unique：同一 user 仅允许一个未删除的 active 套餐（防并发重复激活）。
CREATE UNIQUE INDEX IF NOT EXISTS bundlesubscription_unique_active_per_user
    ON bundle_subscriptions (user_id)
    WHERE status = 'active' AND deleted_at IS NULL;

-- 4. unique：(bundle_subscription_id, group_id, model_pattern) 唯一，防重复 usage 行。
CREATE UNIQUE INDEX IF NOT EXISTS bundlesubscriptionusage_unique_sub_group_pattern
    ON bundle_subscription_usages (bundle_subscription_id, group_id, model_pattern);

-- 5. 索引：支撑 BundleExpiryService.BatchUpdateExpiredStatus
--    (WHERE status='active' AND expires_at <= now)。149 既有的 (user_id,status,expires_at)
--    前导列是 user_id，无法高效支撑按 status+expires_at 的全量扫描。
CREATE INDEX IF NOT EXISTS bundlesubscription_status_expires_at
    ON bundle_subscriptions (status, expires_at);
