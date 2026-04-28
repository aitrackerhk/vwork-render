-- 為 users 表添加 employee_number 欄位
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS employee_number VARCHAR(100);

-- 添加註釋
COMMENT ON COLUMN users.employee_number IS '員工編號（自動生成）';

