-- 為用戶表添加出生日期字段
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS birth_date DATE;

-- 簡單索引（若後續要按生日查詢可用）
CREATE INDEX IF NOT EXISTS idx_users_birth_date ON users(birth_date);

