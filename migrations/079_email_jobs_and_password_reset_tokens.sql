-- Email jobs queue + password reset tokens

-- email_jobs: DB-backed queue for outbound emails
CREATE TABLE IF NOT EXISTS email_jobs (
    id BIGSERIAL PRIMARY KEY,

    tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,

    kind VARCHAR(50) NOT NULL, -- e.g. welcome, password_reset
    idempotency_key VARCHAR(200) UNIQUE,

    to_email VARCHAR(255) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    body_text TEXT,
    body_html TEXT,

    status VARCHAR(20) NOT NULL DEFAULT 'queued', -- queued, sending, sent, dead
    attempts INT NOT NULL DEFAULT 0,
    run_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,

    locked_at TIMESTAMP WITH TIME ZONE,
    locked_by VARCHAR(100),

    last_error TEXT,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_email_jobs_status_run_at
    ON email_jobs (status, run_at);

CREATE INDEX IF NOT EXISTS idx_email_jobs_locked_at
    ON email_jobs (locked_at);


-- password_reset_tokens: one-time tokens (store only hash)
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id
    ON password_reset_tokens (user_id);





