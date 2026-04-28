-- 訂閱計劃和訂閱記錄表

-- 訂閱計劃表
CREATE TABLE IF NOT EXISTS subscription_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    price DECIMAL(10,2) NOT NULL,
    yearly_price DECIMAL(10,2),
    interval VARCHAR(20) NOT NULL,
    stripe_price_id VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 訂閱記錄表
CREATE TABLE IF NOT EXISTS subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    plan_id UUID NOT NULL REFERENCES subscription_plans(id),
    stripe_subscription_id VARCHAR(255) UNIQUE,
    status VARCHAR(50) NOT NULL,
    current_period_start TIMESTAMP WITH TIME ZONE,
    current_period_end TIMESTAMP WITH TIME ZONE,
    cancel_at_period_end BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 為 tenants 表添加新字段（如果不存在）
DO $$ 
BEGIN
    -- 添加 trial_expires_at 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name = 'tenants' AND column_name = 'trial_expires_at') THEN
        ALTER TABLE tenants ADD COLUMN trial_expires_at TIMESTAMP WITH TIME ZONE;
    END IF;
    
    -- 添加 subscription_id 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name = 'tenants' AND column_name = 'subscription_id') THEN
        ALTER TABLE tenants ADD COLUMN subscription_id VARCHAR(255);
    END IF;
    
    -- 添加 stripe_customer_id 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name = 'tenants' AND column_name = 'stripe_customer_id') THEN
        ALTER TABLE tenants ADD COLUMN stripe_customer_id VARCHAR(255);
    END IF;
    
    -- 更新 plan 默認值為 'trial'
    ALTER TABLE tenants ALTER COLUMN plan SET DEFAULT 'trial';
END $$;

-- 插入默認訂閱計劃
INSERT INTO subscription_plans (name, display_name, price, yearly_price, interval, is_active) VALUES
('monthly', '月付方案', 380.00, NULL, 'month', true),
('yearly', '年付方案', 250.00, 3000.00, 'year', true)
ON CONFLICT DO NOTHING;





