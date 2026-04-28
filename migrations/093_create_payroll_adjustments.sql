-- payroll_adjustments：薪資附加項目（多 row，可用 % 或實額進行加/減）
CREATE TABLE IF NOT EXISTS payroll_adjustments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    payroll_id UUID NOT NULL REFERENCES payrolls(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL DEFAULT '',
    direction VARCHAR(20) NOT NULL DEFAULT 'add', -- add / subtract
    mode VARCHAR(20) NOT NULL DEFAULT 'fixed',     -- percent / fixed
    rate DECIMAL(10,4) DEFAULT 0,                  -- percent: 0.05 = 5%
    amount DECIMAL(15,2) DEFAULT 0,                -- fixed amount
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_payroll_adjustments_tenant ON payroll_adjustments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payroll_adjustments_payroll ON payroll_adjustments(payroll_id);


