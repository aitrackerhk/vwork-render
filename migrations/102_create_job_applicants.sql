-- HR: 求職者 / 候選人
-- - 支援 profile_pic（用於列表 avatar 顯示）
-- - job_hires 可連接 applicant_id

CREATE TABLE IF NOT EXISTS job_applicants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    vacancy_id uuid NULL,
    candidate_name varchar(255) NOT NULL,
    candidate_last_name varchar(255) NULL,
    email varchar(255) NULL,
    phone varchar(50) NULL,
    profile_pic text NULL,
    status varchar(20) NOT NULL DEFAULT 'applied', -- applied/interview/offered/hired/rejected
    notes text NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_applicants_tenant_id ON job_applicants(tenant_id);
CREATE INDEX IF NOT EXISTS idx_job_applicants_vacancy_id ON job_applicants(vacancy_id);
CREATE INDEX IF NOT EXISTS idx_job_applicants_status ON job_applicants(status);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_job_applicants_tenant') THEN
        ALTER TABLE job_applicants
            ADD CONSTRAINT fk_job_applicants_tenant
            FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
    END IF;
END $$;

-- vacancy 外鍵（如果表已存在）
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='job_vacancies') THEN
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_job_applicants_vacancy') THEN
            ALTER TABLE job_applicants
                ADD CONSTRAINT fk_job_applicants_vacancy
                FOREIGN KEY (vacancy_id) REFERENCES job_vacancies(id) ON DELETE SET NULL;
        END IF;
    END IF;
END $$;

-- job_hires 連接 applicant_id
ALTER TABLE job_hires
    ADD COLUMN IF NOT EXISTS applicant_id uuid NULL;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='job_applicants') THEN
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_job_hires_applicant') THEN
            ALTER TABLE job_hires
                ADD CONSTRAINT fk_job_hires_applicant
                FOREIGN KEY (applicant_id) REFERENCES job_applicants(id) ON DELETE SET NULL;
        END IF;
    END IF;
END $$;


