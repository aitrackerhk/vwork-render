-- 為客戶表添加性別欄位
-- Add gender column to customers table

ALTER TABLE customers
ADD COLUMN IF NOT EXISTS gender VARCHAR(20);

COMMENT ON COLUMN customers.gender IS '性別：male, female, unknown';


