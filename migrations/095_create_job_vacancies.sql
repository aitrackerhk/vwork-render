-- HR: Job vacancies (空缺)

CREATE TABLE IF NOT EXISTS job_vacancies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    title VARCHAR(255) NOT NULL,
    department_id UUID NULL,
    headcount INTEGER NOT NULL DEFAULT 1,
    status VARCHAR(20) NOT NULL DEFAULT 'open', -- open / closed
    description TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_vacancies_tenant_id ON job_vacancies(tenant_id);
CREATE INDEX IF NOT EXISTS idx_job_vacancies_tenant_status ON job_vacancies(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_job_vacancies_tenant_title ON job_vacancies(tenant_id, title);


