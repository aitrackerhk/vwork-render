-- POS 設定表
CREATE TABLE IF NOT EXISTS pos_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    deposit_percent DECIMAL(5,2) DEFAULT 0,
    deposit_fixed DECIMAL(10,2) DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_pos_settings_tenant ON pos_settings(tenant_id);

