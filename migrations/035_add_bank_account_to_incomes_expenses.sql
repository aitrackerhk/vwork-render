-- 為收入和支出記錄添加銀行賬戶字段

-- 為收入記錄添加銀行賬戶ID字段
ALTER TABLE incomes 
ADD COLUMN IF NOT EXISTS bank_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL;

-- 為支出記錄添加銀行賬戶ID字段
ALTER TABLE expenses 
ADD COLUMN IF NOT EXISTS bank_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL;

-- 創建索引以提高查詢性能
CREATE INDEX IF NOT EXISTS idx_incomes_bank_account_id ON incomes(bank_account_id);
CREATE INDEX IF NOT EXISTS idx_expenses_bank_account_id ON expenses(bank_account_id);

