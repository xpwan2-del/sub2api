-- 修正历史 bundle_upgrade 订单的 amount 字段语义：从"升级差价"改为"目标套餐总价"。
-- 背景：feat/bundles-upgrade 分支原将 bundle_upgrade 订单 amount 设为差价
--   （= 目标套餐价 − prorate_credit），偏离 main 主干"amount = 订单总金额"的语义约定。
-- 本次回归主干语义：amount = 目标套餐总价。
-- 推导：老订单 amount（差价）+ prorate_credit（旧套餐剩余价值抵扣）= 目标套餐总价，故补偿加回。
--   迁移后 GetPaidAmountByBundleSub / refundUpgradeToBalance 的 (amount − prorate_credit)
--   即"用户实付差价"，新老订单一致。
-- 非幂等：本 UPDATE 不可重跑（会重复加 prorate_credit），依赖 migrations_runner 按 NNN 序号
--   仅执行一次（SHA256 checksum 锁定）。
-- ⚠️ 部署顺序：本迁移必须【先于】amount 语义回归的代码变更执行，否则
--   GetPaidAmountByBundleSub / refundUpgradeToBalance 的 (amount − prorate_credit) 公式
--   会对未迁移的老订单（amount 仍是差价）算错，导致升级 credit 折算和退款金额错误。

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

UPDATE payment_orders
SET amount = amount + prorate_credit,
    updated_at = NOW()
WHERE order_type = 'bundle_upgrade'
  AND prorate_credit > 0;
