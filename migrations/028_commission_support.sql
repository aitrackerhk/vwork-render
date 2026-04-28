-- 添加佣金支持
-- 1. 为用户表添加 commission_rate 字段
-- 2. 为订单表添加 salesperson_id 和 commission_amount 字段

-- 为用户表添加佣金率字段
ALTER TABLE users ADD COLUMN IF NOT EXISTS commission_rate DECIMAL(5,2) DEFAULT 0;

-- 为订单表添加销售员ID和佣金金额字段
ALTER TABLE orders ADD COLUMN IF NOT EXISTS salesperson_id UUID REFERENCES users(id);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS commission_amount DECIMAL(15,2) DEFAULT 0;

-- 添加索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_orders_salesperson_id ON orders(salesperson_id);
CREATE INDEX IF NOT EXISTS idx_orders_commission_amount ON orders(commission_amount) WHERE commission_amount > 0;

