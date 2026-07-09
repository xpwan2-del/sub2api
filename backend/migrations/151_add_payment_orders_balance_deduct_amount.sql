-- 151_add_payment_orders_balance_deduct_amount.sql
-- Add balance_deduct_amount column for bundle mixed payment (balance + online payment).

ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS balance_deduct_amount decimal(20,2) NOT NULL DEFAULT 0;
