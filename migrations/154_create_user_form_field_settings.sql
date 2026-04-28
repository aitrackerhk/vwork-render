-- 154_create_user_form_field_settings.sql
-- 創建用戶表單欄位設定表：存儲用戶自定義的表單欄位顯示/隱藏、排序和額外欄位

CREATE TABLE IF NOT EXISTS user_form_field_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- 頁面名稱（如 customers, products, orders 等）
    page_name VARCHAR(100) NOT NULL,
    
    -- 欄位設定（JSON 格式）
    -- 格式: {
    --   "fields": [{ "key": "name", "visible": true, "order": 1 }, ...],
    --   "extraFields": [{ "key": "custom_field_1", "label": "自訂欄位", "type": "text" }, ...]
    -- }
    field_config JSONB NOT NULL DEFAULT '{"fields": [], "extraFields": []}',
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- 每個用戶每個租戶每個頁面只有一條設定記錄
    UNIQUE(tenant_id, user_id, page_name)
);

-- 如果表已存在但缺欄位，補上必要欄位與約束（避免 index 建立失敗）
ALTER TABLE user_form_field_settings ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE user_form_field_settings ADD COLUMN IF NOT EXISTS user_id UUID;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'user_form_field_settings_tenant_id_fkey'
    ) THEN
        ALTER TABLE user_form_field_settings
            ADD CONSTRAINT user_form_field_settings_tenant_id_fkey
            FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'user_form_field_settings_user_id_fkey'
    ) THEN
        ALTER TABLE user_form_field_settings
            ADD CONSTRAINT user_form_field_settings_user_id_fkey
            FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'user_form_field_settings_tenant_user_page_uk'
    ) THEN
        ALTER TABLE user_form_field_settings
            ADD CONSTRAINT user_form_field_settings_tenant_user_page_uk
            UNIQUE (tenant_id, user_id, page_name);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_user_form_field_settings_tenant_user ON user_form_field_settings(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_user_form_field_settings_page ON user_form_field_settings(tenant_id, user_id, page_name);

COMMENT ON TABLE user_form_field_settings IS '用戶表單欄位設定表：存儲用戶自定義的表單欄位顯示/隱藏、排序和額外欄位';
COMMENT ON COLUMN user_form_field_settings.page_name IS '頁面名稱，如 customers, products, orders 等';
COMMENT ON COLUMN user_form_field_settings.field_config IS '欄位設定 JSON，包含 fields 陣列和 extraFields 陣列';
