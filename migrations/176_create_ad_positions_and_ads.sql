-- 廣告位置表
CREATE TABLE IF NOT EXISTS ad_positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(100) NOT NULL,
    description TEXT,
    width INT DEFAULT 1920,
    height INT DEFAULT 1080,
    slide_interval INT DEFAULT 5, -- 輪播間隔（秒）
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(tenant_id, code)
);

CREATE INDEX IF NOT EXISTS idx_ad_positions_tenant_id ON ad_positions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ad_positions_code ON ad_positions(code);

-- 廣告表
CREATE TABLE IF NOT EXISTS ads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    ad_position_id UUID NOT NULL REFERENCES ad_positions(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    media_type VARCHAR(50) NOT NULL DEFAULT 'image', -- image, video
    media_url TEXT NOT NULL,
    media_path TEXT, -- 本地文件路徑
    duration INT DEFAULT 5, -- 顯示時長（秒），圖片用 slide_interval，影片用實際時長
    sort_order INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    start_date TIMESTAMP WITH TIME ZONE,
    end_date TIMESTAMP WITH TIME ZONE,
    extra_fields JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_ads_tenant_id ON ads(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ads_ad_position_id ON ads(ad_position_id);
CREATE INDEX IF NOT EXISTS idx_ads_is_active ON ads(is_active);
CREATE INDEX IF NOT EXISTS idx_ads_sort_order ON ads(sort_order);

-- 輪播設定表
CREATE TABLE IF NOT EXISTS carousel_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    ad_position_id UUID NOT NULL REFERENCES ad_positions(id) ON DELETE CASCADE,
    slide_interval INT DEFAULT 5, -- 圖片輪播秒數
    transition_duration INT DEFAULT 500, -- 轉場時間（毫秒）
    auto_update BOOLEAN DEFAULT TRUE, -- 自動更新
    update_interval INT DEFAULT 3600, -- 更新檢查間隔（秒）
    version INT DEFAULT 1, -- 版本號，用於判斷是否需要更新
    last_generated_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, ad_position_id)
);

CREATE INDEX IF NOT EXISTS idx_carousel_settings_tenant_id ON carousel_settings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_carousel_settings_ad_position_id ON carousel_settings(ad_position_id);
