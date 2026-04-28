-- App 配置表 - 儲存租戶的手機 App 設定
-- Migration: 2026-02-02

CREATE TABLE IF NOT EXISTS app_configs (
    id SERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- 基本資訊
    app_name VARCHAR(100) NOT NULL,
    app_description VARCHAR(500),
    package_name VARCHAR(100),
    bundle_id VARCHAR(100),
    
    -- 品牌設定
    primary_color VARCHAR(20) DEFAULT '#FF5722',
    secondary_color VARCHAR(20) DEFAULT '#FFC107',
    logo_url VARCHAR(500),
    splash_url VARCHAR(500),
    
    -- API 設定
    api_base_url VARCHAR(200),
    
    -- 功能開關
    enable_offline BOOLEAN DEFAULT TRUE,
    enable_notifications BOOLEAN DEFAULT TRUE,
    enable_analytics BOOLEAN DEFAULT FALSE,
    
    -- 構建狀態
    build_status VARCHAR(20) DEFAULT 'pending',
    last_build_at TIMESTAMP WITH TIME ZONE,
    build_error_msg VARCHAR(1000),
    android_apk_url VARCHAR(500),
    android_aab_url VARCHAR(500),
    ios_ipa_url VARCHAR(500),
    
    -- 上架資訊
    google_play_url VARCHAR(500),
    app_store_url VARCHAR(500),
    publish_status VARCHAR(20) DEFAULT 'draft',
    
    -- 索引
    UNIQUE(tenant_id)
);

-- 若已存在，補齊欄位
ALTER TABLE app_configs ADD COLUMN IF NOT EXISTS created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE app_configs ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

-- 創建索引
CREATE INDEX IF NOT EXISTS idx_app_configs_tenant_id ON app_configs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_app_configs_build_status ON app_configs(build_status);

-- 構建記錄表 - 追蹤每次構建的詳細信息
CREATE TABLE IF NOT EXISTS app_build_logs (
    id SERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
,
    build_id VARCHAR(50) NOT NULL,
    platform VARCHAR(20) NOT NULL, -- android, ios, both
    status VARCHAR(20) DEFAULT 'pending', -- pending, building, success, failed
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    config_snapshot JSONB, -- 構建時的配置快照
    artifacts JSONB, -- 構建產出的文件 URL
    
    UNIQUE(build_id)
);

-- 若已存在，補齊欄位
ALTER TABLE app_build_logs ADD COLUMN IF NOT EXISTS build_id VARCHAR(50);

CREATE INDEX IF NOT EXISTS idx_app_build_logs_tenant_id ON app_build_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_app_build_logs_build_id ON app_build_logs(build_id);
CREATE INDEX IF NOT EXISTS idx_app_build_logs_status ON app_build_logs(status);

-- 更新觸發器
CREATE OR REPLACE FUNCTION update_app_configs_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_app_configs_updated_at ON app_configs;
CREATE TRIGGER trigger_app_configs_updated_at
    BEFORE UPDATE ON app_configs
    FOR EACH ROW
    EXECUTE FUNCTION update_app_configs_updated_at();
