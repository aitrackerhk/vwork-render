-- 添加產品圖片 URL 字段
ALTER TABLE products ADD COLUMN IF NOT EXISTS image_url VARCHAR(500);

