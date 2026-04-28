-- 152_create_user_menu_settings.sql
-- 創建用戶菜單設定表：存儲用戶自定義的菜單顯示/隱藏和排序

CREATE TABLE IF NOT EXISTS user_menu_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- 菜單設定（JSON 格式）
    -- 格式: [{ "key": "dashboard", "visible": true, "order": 1 }, ...]
    menu_config JSONB NOT NULL DEFAULT '[]',
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- 每個用戶每個租戶只有一條設定記錄
    UNIQUE(tenant_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_menu_settings_tenant_user ON user_menu_settings(tenant_id, user_id);

COMMENT ON TABLE user_menu_settings IS '用戶菜單設定表：存儲用戶自定義的菜單顯示/隱藏和排序';
COMMENT ON COLUMN user_menu_settings.menu_config IS '菜單設定 JSON 陣列，格式: [{ "key": "dashboard", "visible": true, "order": 1 }, ...]';
