-- 添加系統預設收款帳號和系統預設付款帳號字段
ALTER TABLE bank_accounts 
ADD COLUMN IF NOT EXISTS is_default_receiving BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS is_default_payment BOOLEAN DEFAULT false;

-- 創建索引
CREATE INDEX IF NOT EXISTS idx_bank_accounts_is_default_receiving ON bank_accounts(tenant_id, is_default_receiving) WHERE is_default_receiving = true;
CREATE INDEX IF NOT EXISTS idx_bank_accounts_is_default_payment ON bank_accounts(tenant_id, is_default_payment) WHERE is_default_payment = true;

