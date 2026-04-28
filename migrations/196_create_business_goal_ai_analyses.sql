-- Migration 196: Create business_goal_ai_analyses table
-- Stores persistent AI analysis history records for each business goal
-- Each analysis is saved with timestamp, prompt context, AI response, and analysis type

CREATE TABLE IF NOT EXISTS business_goal_ai_analyses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    goal_id UUID NOT NULL REFERENCES business_goals(id) ON DELETE CASCADE,
    -- analysis type: suggestion (auto analysis), chat (user asked question)
    analysis_type VARCHAR(20) NOT NULL DEFAULT 'suggestion',
    -- user's prompt/question (for chat type)
    user_prompt TEXT,
    -- AI response content
    ai_response TEXT NOT NULL,
    -- snapshot of goal state at analysis time
    goal_snapshot JSONB DEFAULT '{}',
    -- who triggered the analysis
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bg_ai_analyses_tenant_id ON business_goal_ai_analyses (tenant_id);
CREATE INDEX IF NOT EXISTS idx_bg_ai_analyses_goal_id ON business_goal_ai_analyses (goal_id);
CREATE INDEX IF NOT EXISTS idx_bg_ai_analyses_created_at ON business_goal_ai_analyses (goal_id, created_at DESC);
