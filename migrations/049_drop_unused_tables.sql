-- 刪除沒用的表：payments 和 invoices
-- 注意：在執行此遷移之前，請確保沒有其他代碼依賴這些表

-- 刪除 payments 表（如果存在）
DROP TABLE IF EXISTS payments CASCADE;

-- 刪除 invoices 表（如果存在）
DROP TABLE IF EXISTS invoices CASCADE;

-- 刪除相關索引（如果存在）
DROP INDEX IF EXISTS idx_payments_tenant_id;
DROP INDEX IF EXISTS idx_invoices_tenant_id;

