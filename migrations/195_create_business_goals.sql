-- Migration 195: Create business_goals and business_goal_trackings tables
-- Business goals allow users to set measurable targets (e.g., 100 orders, $100k revenue)
-- AI can then track progress and provide suggestions

-- business_goals: stores the goal definitions
CREATE TABLE IF NOT EXISTS business_goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    -- goal metric type: order_count, revenue, customer_count, product_sales_qty, service_order_count, custom
    metric_type VARCHAR(50) NOT NULL DEFAULT 'custom',
    -- for product/service specific goals
    target_entity_id UUID,
    -- measurable target
    target_value DECIMAL(18,2) NOT NULL DEFAULT 0,
    current_value DECIMAL(18,2) NOT NULL DEFAULT 0,
    unit VARCHAR(50) NOT NULL DEFAULT '',
    -- time range
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    -- status: active, completed, failed, paused
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    -- priority: low, medium, high
    priority VARCHAR(20) NOT NULL DEFAULT 'medium',
    -- AI suggestions stored as JSONB array
    ai_suggestions JSONB DEFAULT '{}',
    ai_last_analyzed_at TIMESTAMPTZ,
    -- ownership
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    trashed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    extra_fields JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_business_goals_tenant_id ON business_goals (tenant_id);
CREATE INDEX IF NOT EXISTS idx_business_goals_status ON business_goals (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_business_goals_metric_type ON business_goals (tenant_id, metric_type);
CREATE INDEX IF NOT EXISTS idx_business_goals_dates ON business_goals (tenant_id, start_date, end_date);

-- business_goal_trackings: stores periodic progress snapshots
CREATE TABLE IF NOT EXISTS business_goal_trackings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    goal_id UUID NOT NULL REFERENCES business_goals(id) ON DELETE CASCADE,
    -- snapshot value at this point
    tracked_value DECIMAL(18,2) NOT NULL DEFAULT 0,
    -- delta from previous tracking
    delta_value DECIMAL(18,2) NOT NULL DEFAULT 0,
    -- percentage of target achieved
    progress_pct DECIMAL(5,2) NOT NULL DEFAULT 0,
    -- AI note for this tracking point
    ai_note TEXT,
    -- source: auto (system calculated), manual (user input)
    source VARCHAR(20) NOT NULL DEFAULT 'auto',
    tracked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_business_goal_trackings_goal_id ON business_goal_trackings (goal_id);
CREATE INDEX IF NOT EXISTS idx_business_goal_trackings_tenant_id ON business_goal_trackings (tenant_id);
CREATE INDEX IF NOT EXISTS idx_business_goal_trackings_tracked_at ON business_goal_trackings (goal_id, tracked_at);
