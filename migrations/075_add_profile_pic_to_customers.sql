-- 為客戶表添加頭像字段
ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS profile_pic VARCHAR(500);

