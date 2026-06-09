-- 150_rename_bundle_tier_values.sql
-- Rename bundle plan tier values from basic/flagship/enterprise to starter/pro/enterprise.
-- 'enterprise' remains unchanged.

BEGIN;

UPDATE bundle_plans SET tier = 'starter' WHERE tier = 'basic';
UPDATE bundle_plans SET tier = 'pro' WHERE tier = 'flagship';

COMMIT;
