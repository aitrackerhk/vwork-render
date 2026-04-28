-- 創建優惠券條件表（多條件匹配）
CREATE TABLE IF NOT EXISTS coupon_conditions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id UUID NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    condition_type VARCHAR(50) NOT NULL, -- product_quantity, product_amount, member_level, customer
    product_id UUID REFERENCES products(id) ON DELETE SET NULL,
    quantity INTEGER,
    amount DECIMAL(18, 2),
    member_level_id UUID REFERENCES member_levels(id) ON DELETE SET NULL,
    customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
    match_type VARCHAR(20) DEFAULT 'and', -- and, or
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_coupon_conditions_coupon_id ON coupon_conditions(coupon_id);
CREATE INDEX IF NOT EXISTS idx_coupon_conditions_product_id ON coupon_conditions(product_id);
CREATE INDEX IF NOT EXISTS idx_coupon_conditions_member_level_id ON coupon_conditions(member_level_id);
CREATE INDEX IF NOT EXISTS idx_coupon_conditions_customer_id ON coupon_conditions(customer_id);

COMMENT ON TABLE coupon_conditions IS '優惠券多條件匹配表';
COMMENT ON COLUMN coupon_conditions.condition_type IS '條件類型: product_quantity(特定產品數量), product_amount(特定產品金額), member_level(會員等級), customer(特定客戶)';
COMMENT ON COLUMN coupon_conditions.match_type IS '匹配方式: and(所有條件都需滿足), or(任一條件滿足即可)';

