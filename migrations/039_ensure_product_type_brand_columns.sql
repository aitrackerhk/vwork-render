-- 確保 products 表有 product_type_id 和 brand_id 字段

-- 添加 product_type_id 字段（如果不存在）
ALTER TABLE products
ADD COLUMN IF NOT EXISTS product_type_id UUID REFERENCES product_types(id) ON DELETE SET NULL;

-- 添加 brand_id 字段（如果不存在）
ALTER TABLE products
ADD COLUMN IF NOT EXISTS brand_id UUID REFERENCES brands(id) ON DELETE SET NULL;

-- 創建索引以提高查詢性能
CREATE INDEX IF NOT EXISTS idx_products_product_type_id ON products(product_type_id);
CREATE INDEX IF NOT EXISTS idx_products_brand_id ON products(brand_id);

