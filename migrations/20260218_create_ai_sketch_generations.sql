-- AI Sketch Generations table
-- Stores history of AI-generated images from the Vai Sketch tool (prompt + result)

CREATE TABLE IF NOT EXISTS ai_sketch_generations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sketch_id       UUID REFERENCES ai_sketches(id) ON DELETE SET NULL,
    prompt          TEXT NOT NULL,
    result_image    TEXT DEFAULT '',
    model           VARCHAR(100) DEFAULT '',
    status          VARCHAR(20) DEFAULT 'completed',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for user-scoped queries and chronological listing
CREATE INDEX IF NOT EXISTS idx_ai_sketch_generations_tenant_user ON ai_sketch_generations(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_ai_sketch_generations_created_at ON ai_sketch_generations(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_sketch_generations_sketch_id ON ai_sketch_generations(sketch_id);
