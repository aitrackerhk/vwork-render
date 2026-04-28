-- Create payroll adjustment presets (HR)
-- 用於「薪資附加項目」的常用 preset 管理

CREATE TABLE IF NOT EXISTS payroll_adjustment_presets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    direction VARCHAR(20) NOT NULL DEFAULT 'add', -- add / subtract
    mode VARCHAR(20) NOT NULL DEFAULT 'fixed',    -- fixed / percent
    rate_percent NUMERIC(10, 4) NOT NULL DEFAULT 0, -- percent number, e.g. 5 means 5%
    amount NUMERIC(12, 2) NOT NULL DEFAULT 0,        -- fixed amount
    status VARCHAR(20) NOT NULL DEFAULT 'active',    -- active / inactive
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payroll_adjustment_presets_tenant_id ON payroll_adjustment_presets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payroll_adjustment_presets_tenant_name ON payroll_adjustment_presets(tenant_id, name);


