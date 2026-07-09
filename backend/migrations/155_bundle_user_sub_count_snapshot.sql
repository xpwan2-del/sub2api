SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS daily_limit_count   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_limit_count  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_limit_count INTEGER NOT NULL DEFAULT 0;
