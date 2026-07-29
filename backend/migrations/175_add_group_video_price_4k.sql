-- 为 groups 表新增 4K 视频生成每秒单价列，与现有 video_price_480p/720p/1080p 并列。
-- 精度对齐计费链路：DECIMAL(20,8)（对齐 channel_model_pricing / video_price_1080p）。
-- 幂等：ADD COLUMN IF NOT EXISTS，重跑无副作用。

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE groups ADD COLUMN IF NOT EXISTS video_price_4k DECIMAL(20,8);
COMMENT ON COLUMN groups.video_price_4k IS '4K 视频生成每秒单价 (USD/s)';
