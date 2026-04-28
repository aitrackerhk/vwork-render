-- 預留編號表
CREATE TABLE IF NOT EXISTS reserved_numbers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    field_name VARCHAR(100) NOT NULL,
    field_value VARCHAR(255) NOT NULL,
    page_name VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, field_name, field_value)
);

CREATE INDEX IF NOT EXISTS idx_reserved_numbers_tenant ON reserved_numbers(tenant_id);
CREATE INDEX IF NOT EXISTS idx_reserved_numbers_field ON reserved_numbers(tenant_id, field_name, field_value);

