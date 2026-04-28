-- 產品稅 / 服務稅
-- 用於：產品/服務關聯稅項、在訂單/服務單中預設包含、並可生成支出記錄

-- ============================================
-- 產品稅 (product_taxes)
-- ============================================
CREATE TABLE IF NOT EXISTS product_taxes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    tax_mode VARCHAR(20) NOT NULL DEFAULT 'percent', -- percent / fixed
    tax_value DECIMAL(15,4) NOT NULL DEFAULT 0,
    -- 預設包含於哪些單據：例如 ["order"] 或 ["order","service_order"]
    default_include JSONB DEFAULT '[]',
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_product_taxes_tenant_id ON product_taxes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_product_taxes_status ON product_taxes(status);

-- 產品 - 稅 關聯
CREATE TABLE IF NOT EXISTS product_tax_relations (
    product_id UUID REFERENCES products(id) ON DELETE CASCADE,
    tax_id UUID REFERENCES product_taxes(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(product_id, tax_id)
);

CREATE INDEX IF NOT EXISTS idx_product_tax_relations_tax_id ON product_tax_relations(tax_id);

-- ============================================
-- 服務稅 (service_taxes)
-- ============================================
CREATE TABLE IF NOT EXISTS service_taxes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    tax_mode VARCHAR(20) NOT NULL DEFAULT 'percent', -- percent / fixed
    tax_value DECIMAL(15,4) NOT NULL DEFAULT 0,
    -- 預設包含於哪些單據：例如 ["service_order"] 或 ["order","service_order"]
    default_include JSONB DEFAULT '[]',
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_service_taxes_tenant_id ON service_taxes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_service_taxes_status ON service_taxes(status);

-- 服務 - 稅 關聯
CREATE TABLE IF NOT EXISTS service_tax_relations (
    service_id UUID REFERENCES services(id) ON DELETE CASCADE,
    tax_id UUID REFERENCES service_taxes(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(service_id, tax_id)
);

CREATE INDEX IF NOT EXISTS idx_service_tax_relations_tax_id ON service_tax_relations(tax_id);

