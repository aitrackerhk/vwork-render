-- 為 customers 表添加 is_supplier 欄位
ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS is_supplier BOOLEAN DEFAULT false;

-- 創建索引以優化供應商查詢
CREATE INDEX IF NOT EXISTS idx_customers_is_supplier ON customers(tenant_id, is_supplier) WHERE is_supplier = true;

