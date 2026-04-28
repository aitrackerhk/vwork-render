-- 為 users 表添加 profile_pic 欄位（用戶頭像 URL）
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS profile_pic VARCHAR(500);

COMMENT ON COLUMN users.profile_pic IS '用戶頭像 URL';


