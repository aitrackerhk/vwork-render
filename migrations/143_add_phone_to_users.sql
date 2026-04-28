-- 為用戶表添加電話字段
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS phone VARCHAR(50);

