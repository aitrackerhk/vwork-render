-- 添加服務印花設定欄位到 stamp_settings
ALTER TABLE stamp_settings ADD COLUMN IF NOT EXISTS service_stamp_enabled BOOLEAN DEFAULT false;
ALTER TABLE stamp_settings ADD COLUMN IF NOT EXISTS service_stamp_count INTEGER DEFAULT 1;
ALTER TABLE stamp_settings ADD COLUMN IF NOT EXISTS service_stamp_daily_limit INTEGER;

-- 印花獲取服務設定 (哪些服務可以獲得印花)
CREATE TABLE IF NOT EXISTS stamp_earning_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    stamp_setting_id UUID NOT NULL REFERENCES stamp_settings(id) ON DELETE CASCADE,
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    stamp_count INTEGER DEFAULT 1, -- 購買此服務獲得幾個印花
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, stamp_setting_id, service_id)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_stamp_earning_services_tenant ON stamp_earning_services(tenant_id);
CREATE INDEX IF NOT EXISTS idx_stamp_earning_services_setting ON stamp_earning_services(stamp_setting_id);
CREATE INDEX IF NOT EXISTS idx_stamp_earning_services_service ON stamp_earning_services(service_id);
