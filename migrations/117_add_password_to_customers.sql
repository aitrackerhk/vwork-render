-- 添加密碼字段到 customers 表（用於網店登入）
ALTER TABLE customers
ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255) DEFAULT NULL;

COMMENT ON COLUMN customers.password_hash IS '客戶密碼哈希（用於網店登入）';

CREATE INDEX IF NOT EXISTS idx_customers_email ON customers(email) WHERE email IS NOT NULL;




