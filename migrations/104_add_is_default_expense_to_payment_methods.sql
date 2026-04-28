-- 付款方式：新增「系統預設支出付款方法」
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'payment_methods'
          AND column_name = 'is_default_expense'
    ) THEN
        ALTER TABLE payment_methods
            ADD COLUMN is_default_expense BOOLEAN DEFAULT FALSE;
        CREATE INDEX IF NOT EXISTS idx_payment_methods_default_expense ON payment_methods(is_default_expense);
    END IF;
END $$;


