-- 訂單設置相關表

-- 付款方式表
CREATE TABLE IF NOT EXISTS payment_methods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50) NOT NULL,
    is_default BOOLEAN DEFAULT false,
    status VARCHAR(50) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, code)
);

-- 運送方式表
CREATE TABLE IF NOT EXISTS shipping_methods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50) NOT NULL,
    requires_shipping BOOLEAN DEFAULT false,
    is_default BOOLEAN DEFAULT false,
    status VARCHAR(50) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, code)
);

-- 物流公司表
CREATE TABLE IF NOT EXISTS logistics_companies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    base_fee DECIMAL(15, 2) DEFAULT 0,
    per_item_fee DECIMAL(15, 2) DEFAULT 0,
    per_weight_fee DECIMAL(15, 2) DEFAULT 0,
    per_area_fee DECIMAL(15, 2) DEFAULT 0,
    status VARCHAR(50) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 為產品表添加重量和面積字段
ALTER TABLE products ADD COLUMN IF NOT EXISTS weight DECIMAL(10, 2) DEFAULT 0;
ALTER TABLE products ADD COLUMN IF NOT EXISTS area DECIMAL(10, 2) DEFAULT 0;

-- 為訂單表添加運送方式字段
ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipping_method_id UUID REFERENCES shipping_methods(id);

-- 插入默認付款方式（現金）
INSERT INTO payment_methods (tenant_id, name, code, is_default, status)
SELECT id, '現金', 'cash', true, 'active' FROM tenants
ON CONFLICT (tenant_id, code) DO NOTHING;

-- 插入默認運送方式
INSERT INTO shipping_methods (tenant_id, name, code, requires_shipping, is_default, status)
SELECT id, '店取', 'pickup', false, true, 'active' FROM tenants
ON CONFLICT (tenant_id, code) DO NOTHING;

INSERT INTO shipping_methods (tenant_id, name, code, requires_shipping, is_default, status)
SELECT id, '送貨上門', 'delivery', true, false, 'active' FROM tenants
ON CONFLICT (tenant_id, code) DO NOTHING;

