-- Lead Finder: AI-powered prospect search system
-- 自動搵客系統

CREATE TABLE IF NOT EXISTS lead_finder_searches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_by_id UUID NOT NULL,
    keywords TEXT DEFAULT '',
    target_industry VARCHAR(255) DEFAULT '',
    ai_analysis TEXT DEFAULT '',
    product_id UUID,
    region VARCHAR(255) DEFAULT '',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    result_count INT DEFAULT 0,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lead_finder_searches_tenant ON lead_finder_searches(tenant_id);
CREATE INDEX IF NOT EXISTS idx_lead_finder_searches_status ON lead_finder_searches(tenant_id, status);

CREATE TABLE IF NOT EXISTS lead_finder_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    search_id UUID NOT NULL REFERENCES lead_finder_searches(id) ON DELETE CASCADE,
    company_name VARCHAR(500) DEFAULT '',
    website VARCHAR(1000) DEFAULT '',
    website_domain VARCHAR(255) DEFAULT '',
    phone VARCHAR(255) DEFAULT '',
    normalized_phone VARCHAR(50) DEFAULT '',
    email VARCHAR(500) DEFAULT '',
    address TEXT DEFAULT '',
    description TEXT DEFAULT '',
    source_url VARCHAR(2000) DEFAULT '',
    source_title VARCHAR(1000) DEFAULT '',
    relevance INT DEFAULT 0,
    status VARCHAR(50) NOT NULL DEFAULT 'new',
    converted_to_customer_id UUID,
    notes TEXT DEFAULT '',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lead_finder_results_tenant ON lead_finder_results(tenant_id);
CREATE INDEX IF NOT EXISTS idx_lead_finder_results_search ON lead_finder_results(search_id);
CREATE INDEX IF NOT EXISTS idx_lead_finder_results_status ON lead_finder_results(tenant_id, status);

-- Dedup indexes: partial indexes (only where value is non-empty) for fast duplicate lookups
CREATE INDEX IF NOT EXISTS idx_lead_finder_results_domain ON lead_finder_results(tenant_id, website_domain) WHERE website_domain != '';
CREATE INDEX IF NOT EXISTS idx_lead_finder_results_phone ON lead_finder_results(tenant_id, normalized_phone) WHERE normalized_phone != '';
CREATE INDEX IF NOT EXISTS idx_lead_finder_results_email ON lead_finder_results(tenant_id, lower(email)) WHERE email != '';
