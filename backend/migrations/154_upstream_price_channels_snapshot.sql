-- 154: applied channels snapshot — 批量 follow_cost 记录所有命中 channel 的 prev 价，供撤销遍历恢复。
-- 单条 Apply（applied_channel_id 单值）不受影响；本字段仅在批量 apply 时填充。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE upstream_price_changes
    ADD COLUMN IF NOT EXISTS applied_channels_snapshot JSONB NULL;
