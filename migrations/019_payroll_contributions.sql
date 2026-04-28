-- Payroll 供款明細表
CREATE TABLE IF NOT EXISTS payroll_contributions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    payroll_id UUID NOT NULL REFERENCES payrolls(id) ON DELETE CASCADE,
    payer VARCHAR(20) NOT NULL, -- employee, employer
    mode VARCHAR(20) NOT NULL,  -- percent, fixed
    rate DECIMAL(10,4) DEFAULT 0, -- 例如 0.05 = 5%
    amount DECIMAL(15,2) DEFAULT 0, -- 固定金額
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_payroll_contrib_payroll ON payroll_contributions(payroll_id);
CREATE INDEX IF NOT EXISTS idx_payroll_contrib_tenant ON payroll_contributions(tenant_id);

