-- 印花設定表
CREATE TABLE IF NOT EXISTS stamp_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL DEFAULT '印花活動',
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- active, inactive
    
    -- 購買特定產品獲得印花設定
    product_stamp_enabled BOOLEAN DEFAULT false,
    product_stamp_count INTEGER DEFAULT 1, -- 每次獲得幾個印花
    product_stamp_daily_limit INTEGER, -- 每日每產品獲得上限 (NULL = 無上限)
    
    -- 購買特定金額獲得印花設定
    amount_stamp_enabled BOOLEAN DEFAULT false,
    amount_per_stamp DECIMAL(18,2) DEFAULT 100.00, -- 每消費多少金額獲得1印花
    amount_stamp_daily_limit INTEGER, -- 每日獲得上限 (NULL = 無上限)
    
    valid_from TIMESTAMP WITH TIME ZONE,
    valid_to TIMESTAMP WITH TIME ZONE,
    
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 印花獲取產品設定 (哪些產品可以獲得印花)
CREATE TABLE IF NOT EXISTS stamp_earning_products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    stamp_setting_id UUID NOT NULL REFERENCES stamp_settings(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    stamp_count INTEGER DEFAULT 1, -- 購買此產品獲得幾個印花
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, stamp_setting_id, product_id)
);

-- 印花可換購產品設定
CREATE TABLE IF NOT EXISTS stamp_redeemable_products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    stamp_setting_id UUID NOT NULL REFERENCES stamp_settings(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    stamps_required INTEGER NOT NULL DEFAULT 10, -- 需要多少印花換購
    quantity_limit INTEGER, -- 每次可換數量上限 (NULL = 無上限)
    daily_limit INTEGER, -- 每日可換上限 (NULL = 無上限)
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- active, inactive
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, stamp_setting_id, product_id)
);

-- 客戶印花記錄表
CREATE TABLE IF NOT EXISTS stamp_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    stamp_setting_id UUID REFERENCES stamp_settings(id) ON DELETE SET NULL,
    record_type VARCHAR(20) NOT NULL, -- earn (獲得), redeem (兌換)
    stamp_count INTEGER NOT NULL, -- 正數=獲得, 負數=使用
    balance_after INTEGER NOT NULL DEFAULT 0, -- 操作後餘額
    
    -- 來源記錄
    source_type VARCHAR(50), -- order, service_order, manual, expired
    source_id UUID, -- 關聯的訂單/服務單 ID
    product_id UUID REFERENCES products(id) ON DELETE SET NULL, -- 關聯產品
    
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL
);

-- 客戶印花餘額表 (用於快速查詢)
CREATE TABLE IF NOT EXISTS customer_stamp_balances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    stamp_setting_id UUID NOT NULL REFERENCES stamp_settings(id) ON DELETE CASCADE,
    balance INTEGER NOT NULL DEFAULT 0,
    total_earned INTEGER NOT NULL DEFAULT 0,
    total_redeemed INTEGER NOT NULL DEFAULT 0,
    last_earned_at TIMESTAMP WITH TIME ZONE,
    last_redeemed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, customer_id, stamp_setting_id)
);

-- 每日印花獲取記錄 (用於追蹤每日限制)
CREATE TABLE IF NOT EXISTS stamp_daily_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    stamp_setting_id UUID NOT NULL REFERENCES stamp_settings(id) ON DELETE CASCADE,
    record_date DATE NOT NULL,
    
    -- 產品印花
    product_id UUID REFERENCES products(id) ON DELETE SET NULL,
    product_stamps_earned INTEGER DEFAULT 0,
    
    -- 金額印花
    amount_stamps_earned INTEGER DEFAULT 0,
    
    -- 兌換記錄
    redeemed_product_id UUID REFERENCES products(id) ON DELETE SET NULL,
    redeemed_count INTEGER DEFAULT 0,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 創建索引
CREATE INDEX IF NOT EXISTS idx_stamp_settings_tenant ON stamp_settings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_stamp_settings_status ON stamp_settings(status);
CREATE INDEX IF NOT EXISTS idx_stamp_earning_products_tenant ON stamp_earning_products(tenant_id);
CREATE INDEX IF NOT EXISTS idx_stamp_earning_products_setting ON stamp_earning_products(stamp_setting_id);
CREATE INDEX IF NOT EXISTS idx_stamp_redeemable_products_tenant ON stamp_redeemable_products(tenant_id);
CREATE INDEX IF NOT EXISTS idx_stamp_redeemable_products_setting ON stamp_redeemable_products(stamp_setting_id);
CREATE INDEX IF NOT EXISTS idx_stamp_records_tenant ON stamp_records(tenant_id);
CREATE INDEX IF NOT EXISTS idx_stamp_records_customer ON stamp_records(customer_id);
CREATE INDEX IF NOT EXISTS idx_stamp_records_setting ON stamp_records(stamp_setting_id);
CREATE INDEX IF NOT EXISTS idx_stamp_records_type ON stamp_records(record_type);
CREATE INDEX IF NOT EXISTS idx_stamp_records_source ON stamp_records(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_customer_stamp_balances_tenant ON customer_stamp_balances(tenant_id);
CREATE INDEX IF NOT EXISTS idx_customer_stamp_balances_customer ON customer_stamp_balances(customer_id);
CREATE INDEX IF NOT EXISTS idx_stamp_daily_records_tenant ON stamp_daily_records(tenant_id);
CREATE INDEX IF NOT EXISTS idx_stamp_daily_records_customer_date ON stamp_daily_records(customer_id, record_date);
