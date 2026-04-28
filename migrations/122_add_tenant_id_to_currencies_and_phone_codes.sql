-- 為 currencies 表添加 tenant_id
ALTER TABLE currencies ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- 刪除舊的唯一約束（如果是約束）
ALTER TABLE currencies DROP CONSTRAINT IF EXISTS currencies_code_key;
-- 刪除舊的唯一索引（如果是索引）
DROP INDEX IF EXISTS currencies_code_key;

-- 創建新的唯一索引（tenant_id + code）
CREATE UNIQUE INDEX IF NOT EXISTS idx_currencies_tenant_code ON currencies(tenant_id, code);

-- 為 phone_country_codes 表添加 tenant_id
ALTER TABLE phone_country_codes ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- 刪除舊的唯一約束（如果是約束）
ALTER TABLE phone_country_codes DROP CONSTRAINT IF EXISTS phone_country_codes_code_key;
-- 刪除舊的唯一索引（如果是索引）
DROP INDEX IF EXISTS phone_country_codes_code_key;

-- 創建新的唯一索引（tenant_id + code）
CREATE UNIQUE INDEX IF NOT EXISTS idx_phone_country_codes_tenant_code ON phone_country_codes(tenant_id, code);

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_currencies_tenant_id ON currencies(tenant_id);
CREATE INDEX IF NOT EXISTS idx_phone_country_codes_tenant_id ON phone_country_codes(tenant_id);

