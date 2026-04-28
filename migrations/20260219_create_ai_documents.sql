-- AI Documents table
-- Stores AI-generated documents (docx, xlsx, pptx) for download

CREATE TABLE IF NOT EXISTS ai_documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title           VARCHAR(255) NOT NULL,
    prompt          TEXT NOT NULL,
    doc_type        VARCHAR(20) NOT NULL DEFAULT 'docx',
    file_path       VARCHAR(500) DEFAULT '',
    file_url        VARCHAR(500) DEFAULT '',
    file_size       BIGINT DEFAULT 0,
    model           VARCHAR(100) DEFAULT '',
    status          VARCHAR(20) DEFAULT 'pending',
    error_message   TEXT DEFAULT '',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at    TIMESTAMP WITH TIME ZONE
);

-- Indexes for user-scoped queries and chronological listing
CREATE INDEX IF NOT EXISTS idx_ai_documents_tenant_user ON ai_documents(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_ai_documents_created_at ON ai_documents(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_documents_status ON ai_documents(status);
