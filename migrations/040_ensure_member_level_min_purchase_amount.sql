-- 確保 member_levels 表有 min_purchase_amount 字段
ALTER TABLE member_levels
ADD COLUMN IF NOT EXISTS min_purchase_amount DECIMAL(10,2) DEFAULT 0.00;

