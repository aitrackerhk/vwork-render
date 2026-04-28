-- 文件設定表（發票/發貨單/合約 的條款/備註）
CREATE TABLE IF NOT EXISTS document_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    document_type VARCHAR(50) NOT NULL, -- invoice, shipping_note, contract
    terms TEXT, -- 條款
    notes TEXT, -- 備註
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, document_type)
);

CREATE INDEX IF NOT EXISTS idx_document_settings_tenant_id ON document_settings(tenant_id);







