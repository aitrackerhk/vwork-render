-- 保險處理：補充 bank_accounts 缺失的預設收/付欄位
ALTER TABLE bank_accounts
ADD COLUMN IF NOT EXISTS is_default_receiving BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS is_default_payment   BOOLEAN DEFAULT false;

-- 為預設收/付欄位建立索引（只對 true 建索引以提高查詢效率）
CREATE INDEX IF NOT EXISTS idx_bank_accounts_is_default_receiving
    ON bank_accounts(tenant_id, is_default_receiving)
    WHERE is_default_receiving = true;

CREATE INDEX IF NOT EXISTS idx_bank_accounts_is_default_payment
    ON bank_accounts(tenant_id, is_default_payment)
    WHERE is_default_payment = true;

