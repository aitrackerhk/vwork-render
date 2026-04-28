-- Auto Outreach: fully automated lead finding + outreach scheduling
-- 自動外展：全自動搵客 + 定時發送

CREATE TABLE IF NOT EXISTS auto_outreach_campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_by_id UUID NOT NULL,

    -- Campaign settings
    name VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT DEFAULT '',

    -- Lead search parameters
    search_keywords TEXT DEFAULT '',
    search_region VARCHAR(255) DEFAULT '',
    product_id UUID,

    -- Outreach channel: email, whatsapp, both
    channel VARCHAR(50) NOT NULL DEFAULT 'email',

    -- Message content
    email_subject VARCHAR(500) DEFAULT '',
    message_content TEXT DEFAULT '',

    -- Schedule control
    is_active BOOLEAN NOT NULL DEFAULT true,
    interval_minutes INT NOT NULL DEFAULT 60,
    max_leads_per_run INT NOT NULL DEFAULT 10,
    max_sends_per_run INT NOT NULL DEFAULT 10,
    total_sent_count INT NOT NULL DEFAULT 0,
    total_leads_found INT NOT NULL DEFAULT 0,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    last_run_status VARCHAR(50) DEFAULT '',
    last_run_message TEXT DEFAULT '',
    consecutive_fails INT NOT NULL DEFAULT 0,

    -- Status: active, paused, completed, quota_exceeded
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auto_outreach_campaigns_tenant ON auto_outreach_campaigns(tenant_id);
CREATE INDEX IF NOT EXISTS idx_auto_outreach_campaigns_active ON auto_outreach_campaigns(is_active, next_run_at) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_auto_outreach_campaigns_status ON auto_outreach_campaigns(tenant_id, status);

CREATE TABLE IF NOT EXISTS auto_outreach_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    campaign_id UUID NOT NULL REFERENCES auto_outreach_campaigns(id) ON DELETE CASCADE,

    leads_found INT NOT NULL DEFAULT 0,
    emails_sent INT NOT NULL DEFAULT 0,
    whats_app_sent INT NOT NULL DEFAULT 0,
    fail_count INT NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL DEFAULT '',
    message TEXT DEFAULT '',
    quota_used BIGINT NOT NULL DEFAULT 0,
    quota_remaining BIGINT NOT NULL DEFAULT 0,

    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auto_outreach_logs_tenant ON auto_outreach_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_auto_outreach_logs_campaign ON auto_outreach_logs(campaign_id);
