SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- 目标上游分组 key(对应 new-api group_ratio 的 key,如 default/vip)。
-- 空字符串 = 不过滤,SyncNow 全量同步上游所有模型(向后兼容)。
-- 非空时 SyncNow 仅保留 enable_groups 命中该 key 的模型(enable_groups 为空视为全分组可用)。
ALTER TABLE upstream_source_configs ADD COLUMN IF NOT EXISTS target_upstream_group TEXT NOT NULL DEFAULT '';
