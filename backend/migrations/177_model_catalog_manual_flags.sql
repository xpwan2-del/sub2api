-- 模型广场运营：NEW / featured 改为手动持久化开关
-- is_new：手动 NEW 标签（替代 first_seen_at 时间窗自动判定）
-- featured：手动精选开关（主控；featured_until 保留为可选到期）
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE model_catalog_displays ADD COLUMN IF NOT EXISTS is_new BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE model_catalog_displays ADD COLUMN IF NOT EXISTS featured BOOLEAN NOT NULL DEFAULT FALSE;

-- 平滑回填：旧 featured_until 未到期的行置 featured=true，避免升级丢标签。
-- 仅靠 custom_tags 勾 featured（无日期）的旧行本就不会在公开页显示（历史 bug），此处不回填。
UPDATE model_catalog_displays
SET featured = TRUE
WHERE featured_until IS NOT NULL AND featured_until > now();
