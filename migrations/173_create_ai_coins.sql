-- AI Coins 系統
-- 用於追蹤和計費 AI 功能使用

-- 租戶 AI Coins 帳戶
CREATE TABLE IF NOT EXISTS tenant_ai_coins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    balance INTEGER NOT NULL DEFAULT 0,
    monthly_allotment INTEGER NOT NULL DEFAULT 0,
    monthly_used INTEGER NOT NULL DEFAULT 0,
    monthly_reset_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT (date_trunc('month', now()) + interval '1 month'),
    purchased_balance INTEGER NOT NULL DEFAULT 0,
    total_purchased INTEGER NOT NULL DEFAULT 0,
    total_used INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- AI Coins 交易記錄
CREATE TABLE IF NOT EXISTS ai_coins_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID,
    type VARCHAR(50) NOT NULL, -- purchase, monthly_reset, consume, refund, bonus, expire
    amount INTEGER NOT NULL, -- 正數=增加, 負數=消耗
    balance_after INTEGER NOT NULL,
    description VARCHAR(500),
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_coins_transactions_tenant_id ON ai_coins_transactions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ai_coins_transactions_type ON ai_coins_transactions(type);
CREATE INDEX IF NOT EXISTS idx_ai_coins_transactions_created_at ON ai_coins_transactions(created_at);

-- AI Coins 購買套餐
CREATE TABLE IF NOT EXISTS ai_coins_plans (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    name_en VARCHAR(100),
    coins INTEGER NOT NULL,
    price_hkd INTEGER NOT NULL,
    price_usd INTEGER NOT NULL,
    discount INTEGER DEFAULT 0,
    popular BOOLEAN DEFAULT false,
    active BOOLEAN DEFAULT true,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 插入預設套餐
INSERT INTO ai_coins_plans (id, name, name_en, coins, price_hkd, price_usd, discount, popular, active, sort_order)
VALUES 
    ('small', '小包', 'Small Pack', 100, 20, 3, 0, false, true, 1),
    ('medium', '中包', 'Medium Pack', 300, 50, 7, 15, true, true, 2),
    ('large', '大包', 'Large Pack', 700, 100, 13, 30, false, true, 3),
    ('mega', '超值包', 'Mega Pack', 1600, 200, 26, 37, false, true, 4)
ON CONFLICT (id) DO NOTHING;

-- 為現有租戶創建 AI Coins 帳戶
INSERT INTO tenant_ai_coins (tenant_id, monthly_allotment, monthly_reset_at)
SELECT 
    id,
    CASE plan
        WHEN 'free' THEN 10
        WHEN 'trial' THEN 10
        WHEN 'personal' THEN 100
        WHEN 'basic' THEN 500
        WHEN 'monthly' THEN 500
        WHEN 'yearly' THEN 500
        WHEN 'pro' THEN 1500
        WHEN 'business' THEN 3000
        WHEN 'enterprise' THEN 10000
        ELSE 10
    END,
    date_trunc('month', now()) + interval '1 month'
FROM tenants
WHERE NOT EXISTS (
    SELECT 1 FROM tenant_ai_coins WHERE tenant_ai_coins.tenant_id = tenants.id
);
