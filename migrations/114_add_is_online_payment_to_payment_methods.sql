-- 添加 is_online_payment 字段到 payment_methods 表
ALTER TABLE payment_methods 
ADD COLUMN IF NOT EXISTS is_online_payment BOOLEAN DEFAULT FALSE;

-- 添加注释
COMMENT ON COLUMN payment_methods.is_online_payment IS '網店付款方式（是否在 checkout 元件中顯示）';




