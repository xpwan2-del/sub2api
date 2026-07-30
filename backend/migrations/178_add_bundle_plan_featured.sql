-- 套餐计划新增 featured（是否推荐）字段，用于模型广场/套餐页的推荐标记。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE bundle_plans
    ADD COLUMN IF NOT EXISTS featured BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN bundle_plans.featured IS '是否推荐套餐';
