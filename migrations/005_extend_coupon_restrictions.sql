-- 擴展優惠券限制條件
-- 添加會員等級限制、特定產品數量限制等字段

ALTER TABLE coupons
ADD COLUMN IF NOT EXISTS member_level_id UUID REFERENCES member_levels(id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS min_product_quantity INTEGER DEFAULT NULL,
ADD COLUMN IF NOT EXISTS min_product_amount DECIMAL(18, 2) DEFAULT NULL;

COMMENT ON COLUMN coupons.member_level_id IS '限制特定會員等級使用';
COMMENT ON COLUMN coupons.min_product_quantity IS '最低產品數量要求';
COMMENT ON COLUMN coupons.min_product_amount IS '最低產品金額要求';

