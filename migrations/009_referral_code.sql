-- 添加介绍人referral code功能

-- 在customers表中添加referral_code字段
ALTER TABLE customers ADD COLUMN IF NOT EXISTS referral_code VARCHAR(50);

-- 在orders表中添加referral_code字段
ALTER TABLE orders ADD COLUMN IF NOT EXISTS referral_code VARCHAR(50);

-- 在point_settings表中添加referral_bonus_points字段（介绍人奖励积分）
ALTER TABLE point_settings ADD COLUMN IF NOT EXISTS referral_bonus_points INTEGER DEFAULT 0;

-- 在point_settings表中添加referral_count_policy字段（介绍人积分计算策略）
ALTER TABLE point_settings ADD COLUMN IF NOT EXISTS referral_count_policy VARCHAR(20) DEFAULT 'all';

-- 创建索引（在列存在后）
CREATE INDEX IF NOT EXISTS idx_customers_referral_code ON customers(tenant_id, referral_code) WHERE referral_code IS NOT NULL;

