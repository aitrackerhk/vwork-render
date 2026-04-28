-- 165_create_drafts.sql
-- 用戶草稿（按租戶 + 使用者保存表單草稿資料）

CREATE TABLE IF NOT EXISTS drafts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    page_name VARCHAR(100) NOT NULL,
    key_field_value VARCHAR(255),
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_drafts_tenant_user ON drafts(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_drafts_tenant_user_page ON drafts(tenant_id, user_id, page_name);
CREATE INDEX IF NOT EXISTS idx_drafts_updated_at ON drafts(updated_at DESC);

COMMENT ON TABLE drafts IS '用戶草稿表：按租戶/用戶保存表單草稿資料';
COMMENT ON COLUMN drafts.page_name IS '表單頁面名稱（例如 orders, service-orders）';
COMMENT ON COLUMN drafts.key_field_value IS '表單主鍵欄位值（如 order_number/code）';
COMMENT ON COLUMN drafts.data IS '表單草稿 JSON 資料';
