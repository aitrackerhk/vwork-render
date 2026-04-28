-- Add is_non_inventory flag to products
-- If is_non_inventory is true, all inventory and delivery related functions will skip this product
ALTER TABLE products ADD COLUMN IF NOT EXISTS is_non_inventory BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN products.is_non_inventory IS '非庫存類產品：若為 true，所有庫存和配送相關功能將不計算此產品';
