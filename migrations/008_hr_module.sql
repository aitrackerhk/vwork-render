-- HR 模組表

-- 打卡記錄表
CREATE TABLE IF NOT EXISTS attendances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    clock_in TIMESTAMP,
    clock_out TIMESTAMP,
    break_duration INTEGER DEFAULT 0,
    work_duration INTEGER DEFAULT 0,
    ot_duration INTEGER DEFAULT 0,
    status VARCHAR(50) DEFAULT 'normal',
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, user_id, date)
);

CREATE INDEX IF NOT EXISTS idx_attendances_tenant ON attendances(tenant_id);
CREATE INDEX IF NOT EXISTS idx_attendances_user ON attendances(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_attendances_date ON attendances(tenant_id, date);

-- 請假申請表
CREATE TABLE IF NOT EXISTS leave_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    leave_type VARCHAR(50) NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    days DECIMAL(5,2) NOT NULL,
    reason TEXT,
    status VARCHAR(50) DEFAULT 'pending',
    approved_by UUID REFERENCES users(id),
    approved_at TIMESTAMP,
    reject_reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_leave_requests_tenant ON leave_requests(tenant_id);
CREATE INDEX IF NOT EXISTS idx_leave_requests_user ON leave_requests(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_leave_requests_status ON leave_requests(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_leave_requests_date ON leave_requests(tenant_id, start_date, end_date);

-- 薪資記錄表
CREATE TABLE IF NOT EXISTS payrolls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pay_period VARCHAR(50) NOT NULL,
    base_salary DECIMAL(15,2) DEFAULT 0,
    ot_hours DECIMAL(10,2) DEFAULT 0,
    ot_amount DECIMAL(15,2) DEFAULT 0,
    mpf_employee DECIMAL(15,2) DEFAULT 0,
    mpf_employer DECIMAL(15,2) DEFAULT 0,
    mpf_total DECIMAL(15,2) DEFAULT 0,
    allowances DECIMAL(15,2) DEFAULT 0,
    deductions DECIMAL(15,2) DEFAULT 0,
    gross_salary DECIMAL(15,2) DEFAULT 0,
    net_salary DECIMAL(15,2) DEFAULT 0,
    status VARCHAR(50) DEFAULT 'draft',
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, user_id, pay_period)
);

CREATE INDEX IF NOT EXISTS idx_payrolls_tenant ON payrolls(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payrolls_user ON payrolls(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_payrolls_period ON payrolls(tenant_id, pay_period);

-- 修改 service_staff 表，添加 name 和 phone 字段
ALTER TABLE service_staff ADD COLUMN IF NOT EXISTS name VARCHAR(255);
ALTER TABLE service_staff ADD COLUMN IF NOT EXISTS phone VARCHAR(50);
ALTER TABLE service_staff ALTER COLUMN user_id DROP NOT NULL;










