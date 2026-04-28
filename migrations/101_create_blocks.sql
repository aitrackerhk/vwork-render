-- 區塊管理：blocks（可重用的頁面元件區塊）
CREATE TABLE IF NOT EXISTS blocks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    component_type VARCHAR(100) NOT NULL, -- hero, text, image, button, section, etc.
    component_data JSONB NOT NULL DEFAULT '{}'::jsonb, -- 元件配置數據
    created_by UUID REFERENCES users(id),
    updated_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE, -- 軟刪除時間戳
    extra_fields JSONB DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_blocks_tenant ON blocks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_blocks_name ON blocks(tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_blocks_component_type ON blocks(component_type);
CREATE INDEX IF NOT EXISTS idx_blocks_deleted_at ON blocks(deleted_at);

