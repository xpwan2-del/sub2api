SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- 上游所选分组倍率的最后成功观察值。首次同步仅建立基线，不生成倍率审批。
ALTER TABLE upstream_source_configs
    ADD COLUMN IF NOT EXISTS group_ratio_baseline_key TEXT,
    ADD COLUMN IF NOT EXISTS group_ratio_baseline_value NUMERIC(18,8),
    ADD COLUMN IF NOT EXISTS group_ratio_baseline_observed_at TIMESTAMPTZ;

-- 默认同步目标渠道的全部分组；这里只记录管理员明确排除的分组。
CREATE TABLE IF NOT EXISTS upstream_source_excluded_groups (
    source_config_id BIGINT NOT NULL REFERENCES upstream_source_configs(id) ON DELETE CASCADE,
    group_id         BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source_config_id, group_id)
);
CREATE INDEX IF NOT EXISTS idx_upstream_source_excluded_groups_group
    ON upstream_source_excluded_groups (group_id);

-- 分组倍率使用专用快照，不复用模型价格 ConvertedPrice JSON。
ALTER TABLE upstream_price_change_items
    ADD COLUMN IF NOT EXISTS target_group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS group_rate_change JSONB,
    ADD COLUMN IF NOT EXISTS apply_rate DECIMAL(10,4);

CREATE INDEX IF NOT EXISTS idx_upstream_price_change_items_pending_group
    ON upstream_price_change_items (request_id, target_group_id)
    WHERE kind = 'group_ratio' AND status = 'pending';
