-- 150_rename_bundle_tier_values.sql
-- 重命名套餐层级值：basic → starter，flagship → pro。
-- enterprise 保持不变。
-- 与前端 bundleTiers.ts 和后端 bundle_constants.go 的层级常量对齐。

BEGIN;

UPDATE bundle_plans SET tier = 'starter' WHERE tier = 'basic';
UPDATE bundle_plans SET tier = 'pro' WHERE tier = 'flagship';

COMMIT;
