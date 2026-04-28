-- vOffice 安裝統計 + 版本管理
-- 記錄每台安裝 vOffice 的電腦資訊

CREATE TABLE IF NOT EXISTS voffice_installations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    machine_id VARCHAR(255) NOT NULL,              -- 機器唯一識別碼 (hardware hash)
    tenant_id UUID REFERENCES tenants(id),          -- 關聯租戶 (可為 NULL, 未登入時)
    user_id UUID REFERENCES users(id),              -- 關聯用戶 (可為 NULL, 未登入時)

    -- 軟體資訊
    app_version VARCHAR(50) NOT NULL DEFAULT '',    -- vOffice 版本號 (e.g. "1.0.0")
    build_number VARCHAR(50) DEFAULT '',            -- Build 編號
    update_channel VARCHAR(20) DEFAULT 'stable',    -- stable / beta / dev

    -- 作業系統
    os_type VARCHAR(20) NOT NULL DEFAULT '',        -- windows / macos / linux
    os_version VARCHAR(100) DEFAULT '',             -- e.g. "Windows 11 23H2", "macOS 14.2"
    os_arch VARCHAR(20) DEFAULT '',                 -- x64 / arm64

    -- 硬體資訊
    cpu_model VARCHAR(200) DEFAULT '',
    cpu_cores INT DEFAULT 0,
    ram_gb INT DEFAULT 0,
    screen_resolution VARCHAR(50) DEFAULT '',       -- e.g. "1920x1080"
    display_count INT DEFAULT 1,

    -- 網路/地理
    ip_address VARCHAR(50) DEFAULT '',
    country VARCHAR(10) DEFAULT '',
    city VARCHAR(100) DEFAULT '',
    language VARCHAR(20) DEFAULT '',                -- 系統語言 e.g. "zh-TW"

    -- 狀態
    is_active BOOLEAN DEFAULT true,
    first_seen_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMP WITH TIME ZONE,

    -- Metadata
    extra_data JSONB DEFAULT '{}',

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 索引
CREATE UNIQUE INDEX IF NOT EXISTS idx_voffice_installations_machine_id ON voffice_installations(machine_id);
CREATE INDEX IF NOT EXISTS idx_voffice_installations_tenant_id ON voffice_installations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_voffice_installations_user_id ON voffice_installations(user_id);
CREATE INDEX IF NOT EXISTS idx_voffice_installations_os_type ON voffice_installations(os_type);
CREATE INDEX IF NOT EXISTS idx_voffice_installations_app_version ON voffice_installations(app_version);
CREATE INDEX IF NOT EXISTS idx_voffice_installations_last_seen ON voffice_installations(last_seen_at);
CREATE INDEX IF NOT EXISTS idx_voffice_installations_is_active ON voffice_installations(is_active);

-- vOffice 版本管理表
CREATE TABLE IF NOT EXISTS voffice_releases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version VARCHAR(50) NOT NULL,                   -- e.g. "1.0.1"
    build_number VARCHAR(50) DEFAULT '',
    channel VARCHAR(20) NOT NULL DEFAULT 'stable',  -- stable / beta / dev
    platform VARCHAR(20) NOT NULL,                  -- windows / macos / linux

    -- 下載資訊
    download_url TEXT NOT NULL,                     -- 安裝包下載 URL
    file_size BIGINT DEFAULT 0,                     -- 檔案大小 (bytes)
    checksum VARCHAR(128) DEFAULT '',               -- SHA-256 checksum

    -- 版本資訊
    release_notes TEXT DEFAULT '',
    min_os_version VARCHAR(50) DEFAULT '',           -- 最低作業系統版本要求
    is_mandatory BOOLEAN DEFAULT false,              -- 是否強制更新
    is_latest BOOLEAN DEFAULT false,                 -- 是否為最新版

    published_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_voffice_releases_version ON voffice_releases(version);
CREATE INDEX IF NOT EXISTS idx_voffice_releases_channel_platform ON voffice_releases(channel, platform);
CREATE INDEX IF NOT EXISTS idx_voffice_releases_is_latest ON voffice_releases(is_latest);
