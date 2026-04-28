-- 單據設定：系統自動生成設定
-- - auto_generate_commission：自動生成傭金支出
-- - auto_generate_order_taxes：自動生成訂單稅務支出
-- - auto_generate_service_taxes：自動生成服務稅務支出

CREATE TABLE IF NOT EXISTS document_auto_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    auto_generate_commission BOOLEAN NOT NULL DEFAULT TRUE,
    auto_generate_order_taxes BOOLEAN NOT NULL DEFAULT TRUE,
    auto_generate_service_taxes BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_document_auto_settings_tenant_id ON document_auto_settings(tenant_id);


