-- 153: upstream price hardening — apply 前实际值快照 + 撤销标记
-- 支撑第 4 项（覆盖保护 + UI 撤销回滚）。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE upstream_price_changes
    ADD COLUMN IF NOT EXISTS applied_prev_input_price NUMERIC(20,12) NULL,
    ADD COLUMN IF NOT EXISTS applied_prev_output_price NUMERIC(20,12) NULL,
    ADD COLUMN IF NOT EXISTS applied_channel_id BIGINT NULL,
    ADD COLUMN IF NOT EXISTS prev_multiplier DECIMAL(10,4) NULL,
    ADD COLUMN IF NOT EXISTS reverted_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS reverted_by BIGINT NULL;
