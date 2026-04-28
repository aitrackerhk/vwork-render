-- 為 products 表添加 allow_backorder 字段
ALTER TABLE products
ADD COLUMN IF NOT EXISTS allow_backorder BOOLEAN DEFAULT false;

