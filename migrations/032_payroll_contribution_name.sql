-- 添加款項名稱字段到 payroll_contributions 表
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'payroll_contributions' AND column_name = 'name') THEN
        ALTER TABLE payroll_contributions ADD COLUMN name VARCHAR(255) NOT NULL DEFAULT '';
    END IF;
END $$;







