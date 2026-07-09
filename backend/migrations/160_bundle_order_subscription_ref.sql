-- 160_bundle_order_subscription_ref.sql
-- payment_orders 增加 bundle_subscription_id 列：套餐订单激活后回写 BundleSubscription ID，
-- 便于财务对账与追溯（当前套餐不支持退款，仍保留订单↔订阅关联以便审计）。
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS bundle_subscription_id BIGINT DEFAULT NULL;
