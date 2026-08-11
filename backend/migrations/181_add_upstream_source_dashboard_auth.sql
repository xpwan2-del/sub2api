SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE upstream_source_configs
    ADD COLUMN IF NOT EXISTS dashboard_auth_mode VARCHAR(20) NOT NULL DEFAULT 'auto',
    ADD COLUMN IF NOT EXISTS dashboard_user_id BIGINT;

COMMENT ON COLUMN upstream_source_configs.dashboard_auth_mode IS
    'Dashboard 鉴权模式: auto | bearer | raw | raw_user | bearer_user';
COMMENT ON COLUMN upstream_source_configs.dashboard_user_id IS
    '面板用户 ID（raw_user/bearer_user 模式下随 New-Api-User 请求头发送）';
