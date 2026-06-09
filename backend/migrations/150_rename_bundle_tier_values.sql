-- 150_rename_bundle_tier_values.sql
-- Rename bundle plan tier values from basic/flagship/enterprise to starter/pro/enterprise.
-- 'enterprise' remains unchanged.
-- Note: The migration runner wraps this in a transaction, so explicit BEGIN/COMMIT
-- is intentionally omitted to avoid nested-transaction issues with pg 18+.

UPDATE bundle_plans SET tier = 'starter' WHERE tier = 'basic';
UPDATE bundle_plans SET tier = 'pro' WHERE tier = 'flagship';
