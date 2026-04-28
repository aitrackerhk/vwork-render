-- incomes / expenses：新增相關人員（related_user_id）
ALTER TABLE incomes
    ADD COLUMN IF NOT EXISTS related_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_incomes_related_user_id ON incomes(related_user_id);

ALTER TABLE expenses
    ADD COLUMN IF NOT EXISTS related_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_expenses_related_user_id ON expenses(related_user_id);


