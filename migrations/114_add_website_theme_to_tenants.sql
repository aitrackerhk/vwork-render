-- 添加網站主題字段到 tenants 表
ALTER TABLE tenants
ADD COLUMN IF NOT EXISTS website_theme VARCHAR(50) DEFAULT NULL,
ADD COLUMN IF NOT EXISTS website_type VARCHAR(50) DEFAULT NULL, -- 'ecommerce', 'general', null
ADD COLUMN IF NOT EXISTS website_enabled BOOLEAN DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_tenants_website_theme ON tenants(website_theme);
CREATE INDEX IF NOT EXISTS idx_tenants_website_type ON tenants(website_type);

COMMENT ON COLUMN tenants.website_theme IS '網站主題：default, modern, classic 等';
COMMENT ON COLUMN tenants.website_type IS '網站類型：ecommerce（電商）, general（一般）';
COMMENT ON COLUMN tenants.website_enabled IS '是否啟用網站功能';




