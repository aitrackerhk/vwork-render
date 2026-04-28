-- AI Sketches table for Vai Sketch tool
-- Stores user-created sketches with canvas objects, dimensions, and thumbnails

CREATE TABLE IF NOT EXISTS ai_sketches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title           VARCHAR(255) NOT NULL DEFAULT '未命名草圖',
    objects         TEXT DEFAULT '[]',
    canvas_width    INT DEFAULT 800,
    canvas_height   INT DEFAULT 600,
    thumbnail       TEXT DEFAULT '',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for user-scoped queries
CREATE INDEX IF NOT EXISTS idx_ai_sketches_tenant_user ON ai_sketches(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_ai_sketches_updated_at ON ai_sketches(updated_at DESC);
