-- 添加店舖圖片 URL 字段
ALTER TABLE stores ADD COLUMN IF NOT EXISTS image_url VARCHAR(500);

