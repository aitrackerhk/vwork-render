-- Job applicants: worker type + extra_fields for 欄位設定 dynamic forms
ALTER TABLE job_applicants
    ADD COLUMN IF NOT EXISTS worker_type varchar(20) NOT NULL DEFAULT '';

ALTER TABLE job_applicants
    ADD COLUMN IF NOT EXISTS extra_fields jsonb DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_job_applicants_worker_type ON job_applicants(tenant_id, worker_type);
