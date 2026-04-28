-- 163_warehouse_zones_and_inventory_settings.sql
-- 新增倉庫區 (warehouse_zones) 表
-- 新增出入庫設定 (inventory_settings) 表
-- 產品增加預設倉庫區欄位

-- 倉庫區表
CREATE TABLE IF NOT EXISTS warehouse_zones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    warehouse_id UUID NOT NULL REFERENCES warehouses(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(100),
    description TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    status VARCHAR(50) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),
    updated_by UUID REFERENCES users(id),
    trashed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_warehouse_zones_tenant_id ON warehouse_zones(tenant_id);
CREATE INDEX IF NOT EXISTS idx_warehouse_zones_warehouse_id ON warehouse_zones(warehouse_id);

-- 出入庫設定表
CREATE TABLE IF NOT EXISTS inventory_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    requires_outbound BOOLEAN DEFAULT TRUE,
    requires_inbound BOOLEAN DEFAULT TRUE,
    auto_complete_if_no_need BOOLEAN DEFAULT TRUE,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),
    updated_by UUID REFERENCES users(id)
);

-- 產品增加預設倉庫區欄位
ALTER TABLE products ADD COLUMN IF NOT EXISTS default_warehouse_zone_id UUID REFERENCES warehouse_zones(id);
