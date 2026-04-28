-- 倉庫管理
-- 創建倉庫表
CREATE TABLE IF NOT EXISTS warehouses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(100) NOT NULL,
    address TEXT,
    contact_person VARCHAR(255),
    phone VARCHAR(50),
    email VARCHAR(255),
    status VARCHAR(50) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id),
    updated_by UUID REFERENCES users(id),
    UNIQUE(tenant_id, code)
);

CREATE INDEX IF NOT EXISTS idx_warehouses_tenant ON warehouses(tenant_id);
CREATE INDEX IF NOT EXISTS idx_warehouses_code ON warehouses(tenant_id, code);

-- 創建產品倉庫庫存表
CREATE TABLE IF NOT EXISTS product_warehouse_stocks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    warehouse_id UUID NOT NULL REFERENCES warehouses(id) ON DELETE CASCADE,
    quantity INTEGER NOT NULL DEFAULT 0,
    reserved_quantity INTEGER DEFAULT 0,
    last_updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    extra_fields JSONB DEFAULT '{}',
    UNIQUE(tenant_id, product_id, warehouse_id)
);

CREATE INDEX IF NOT EXISTS idx_product_warehouse_stocks_tenant ON product_warehouse_stocks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_product_warehouse_stocks_product ON product_warehouse_stocks(product_id);
CREATE INDEX IF NOT EXISTS idx_product_warehouse_stocks_warehouse ON product_warehouse_stocks(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_product_warehouse_stocks_composite ON product_warehouse_stocks(tenant_id, product_id, warehouse_id);

