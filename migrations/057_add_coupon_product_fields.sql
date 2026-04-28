-- 為 coupons 表添加 min_product_quantity 和 min_product_amount 欄位（如果不存在）
ALTER TABLE coupons
    ADD COLUMN IF NOT EXISTS min_product_quantity INTEGER DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS min_product_amount DECIMAL(18, 2) DEFAULT NULL;

-- 添加註釋
COMMENT ON COLUMN coupons.min_product_quantity IS '最低產品數量要求';
COMMENT ON COLUMN coupons.min_product_amount IS '最低產品金額要求';

