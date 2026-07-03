-- 158_bundle_money_numeric.sql
-- 套餐金额列精度对齐：double precision → numeric/decimal
--
-- 背景：套餐表是全项目唯一违反 CLAUDE.md 精度规范的金额字段群（裸 double precision）。
-- 本次对齐 subscription / payment / usage_log / group 等既有表的 numeric/decimal 模式。
-- 用量累加发生在 SQL 层（ent Add → SET x = x + $1），改 numeric 后加法在 numeric 列上执行，
-- 消除反复累加的浮点漂移；Go 侧仍为 float64（ent 自动 float8↔numeric 转换），API 无变化。
--
-- 精度档位对齐：
--   bundle_plans.price/original_price            → decimal(20,2)  (对齐 subscription_plan.price / payment_order.amount)
--   bundle_plan_group_quotas.*_limit_usd         → decimal(20,8)  (对齐 group.*_limit_usd)
--   bundle_subscription_usages.*_usage_usd       → decimal(20,10) (对齐 user_subscription.*_usage_usd)
--   user_subscriptions.*_limit_usd (bundle 快照) → decimal(20,8)  (对齐 group.*_limit_usd)
--
-- DDL 锁：ALTER COLUMN TYPE 持 AccessExclusiveLock + 表重写；套餐表数据量小，配合锁超时低峰执行。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- bundle_plans: 售价
ALTER TABLE bundle_plans
    ALTER COLUMN price TYPE decimal(20,2) USING price::numeric(20,2),
    ALTER COLUMN original_price TYPE decimal(20,2) USING original_price::numeric(20,2);

-- bundle_plan_group_quotas: 额度上限
ALTER TABLE bundle_plan_group_quotas
    ALTER COLUMN daily_limit_usd   TYPE decimal(20,8) USING daily_limit_usd::numeric(20,8),
    ALTER COLUMN weekly_limit_usd  TYPE decimal(20,8) USING weekly_limit_usd::numeric(20,8),
    ALTER COLUMN monthly_limit_usd TYPE decimal(20,8) USING monthly_limit_usd::numeric(20,8);

-- bundle_subscription_usages: 累计用量
ALTER TABLE bundle_subscription_usages
    ALTER COLUMN daily_usage_usd   TYPE decimal(20,10) USING daily_usage_usd::numeric(20,10),
    ALTER COLUMN weekly_usage_usd  TYPE decimal(20,10) USING weekly_usage_usd::numeric(20,10),
    ALTER COLUMN monthly_usage_usd TYPE decimal(20,10) USING monthly_usage_usd::numeric(20,10);

-- user_subscriptions: 套餐桥接快照限额（149 新增的 bundle 列）
ALTER TABLE user_subscriptions
    ALTER COLUMN daily_limit_usd   TYPE decimal(20,8) USING daily_limit_usd::numeric(20,8),
    ALTER COLUMN weekly_limit_usd  TYPE decimal(20,8) USING weekly_limit_usd::numeric(20,8),
    ALTER COLUMN monthly_limit_usd TYPE decimal(20,8) USING monthly_limit_usd::numeric(20,8);
