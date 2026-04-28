-- HR: Hiring records (聘請)

CREATE TABLE IF NOT EXISTS job_hires (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    vacancy_id UUID NULL,
    candidate_name VARCHAR(255) NOT NULL,
    candidate_last_name VARCHAR(255) NULL,
    email VARCHAR(255) NULL,
    phone VARCHAR(50) NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'applied', -- applied/interview/offered/hired/rejected
    start_date DATE NULL,
    notes TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_hires_tenant_id ON job_hires(tenant_id);
CREATE INDEX IF NOT EXISTS idx_job_hires_tenant_status ON job_hires(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_job_hires_tenant_vacancy ON job_hires(tenant_id, vacancy_id);


