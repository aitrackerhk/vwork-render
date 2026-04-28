-- 添加網站設定字段到 tenants 表
-- 網店開放國家和預設網頁語言將存儲在 extra_fields JSONB 中
-- 如果需要，可以在這裡添加具體的字段

-- 如果需要單獨的字段，可以取消註釋以下行：
-- ALTER TABLE tenants
-- ADD COLUMN IF NOT EXISTS allowed_countries JSONB DEFAULT '[]'::jsonb,
-- ADD COLUMN IF NOT EXISTS default_language VARCHAR(10) DEFAULT NULL;

-- 目前使用 extra_fields 存儲，無需額外字段

