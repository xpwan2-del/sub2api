SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE bundle_subscriptions ADD COLUMN IF NOT EXISTS upgraded_from_id BIGINT NULL;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS source_bundle_subscription_id BIGINT NULL;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS prorate_credit NUMERIC(20,12) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_bundle_subs_upgraded_from ON bundle_subscriptions(upgraded_from_id);
CREATE INDEX IF NOT EXISTS idx_payment_orders_src_bundle ON payment_orders(source_bundle_subscription_id);
