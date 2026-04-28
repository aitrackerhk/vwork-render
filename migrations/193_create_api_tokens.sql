-- System-level API tokens for external service integrations (v01, vOffice, third-party).
-- Holders of a valid token can call vWork API without user login.

CREATE TABLE IF NOT EXISTS api_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    token_hash      VARCHAR(128) NOT NULL,
    token_prefix    VARCHAR(12)  NOT NULL,          -- first 8 chars for display / lookup
    scopes          TEXT         NOT NULL DEFAULT '*',  -- comma-separated, "*" = full access
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',  -- active | revoked
    created_by_id   UUID         NOT NULL REFERENCES users(id) ON DELETE SET NULL,
    last_used_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,                    -- NULL = never expires
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Fast lookup by hash (used on every authenticated request)
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens (token_hash);

-- List tokens per tenant
CREATE INDEX IF NOT EXISTS idx_api_tokens_tenant_id ON api_tokens (tenant_id);

-- Quick filter: active + not expired
CREATE INDEX IF NOT EXISTS idx_api_tokens_status ON api_tokens (status) WHERE status = 'active';
