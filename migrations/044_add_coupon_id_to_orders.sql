-- 為訂單表添加 coupon_id 字段（如果不存在）

-- 添加 coupon_id 字段
ALTER TABLE orders 
ADD COLUMN IF NOT EXISTS coupon_id UUID REFERENCES coupons(id) ON DELETE SET NULL;

-- 創建索引以提高查詢性能
CREATE INDEX IF NOT EXISTS idx_orders_coupon_id ON orders(coupon_id);

