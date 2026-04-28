-- 為產品類型表添加圖片字段

ALTER TABLE product_types
ADD COLUMN IF NOT EXISTS image_url VARCHAR(500);

-- 創建索引（如果需要）
-- CREATE INDEX IF NOT EXISTS idx_product_types_image_url ON product_types(image_url);

