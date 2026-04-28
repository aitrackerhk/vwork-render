-- Create industry templates table
-- 行業模板表：系統級配置，所有租戶共享

CREATE TABLE IF NOT EXISTS industry_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(50) UNIQUE NOT NULL,  -- "retail", "manufacturing", "service", "ecommerce", "logistics", "sme"
    name VARCHAR(255) NOT NULL,
    name_en VARCHAR(255),
    description TEXT,
    description_en TEXT,
    enabled_modules JSONB DEFAULT '[]'::jsonb,  -- 啟用的模塊列表，例如：["inventory", "pos", "hr"]
    default_fields JSONB DEFAULT '{}'::jsonb,  -- 各表的默認動態字段配置
    icon VARCHAR(100),  -- 圖標名稱或路徑
    is_active BOOLEAN DEFAULT true,
    sort_order INTEGER DEFAULT 0,  -- 排序順序
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_industry_templates_code ON industry_templates(code);
CREATE INDEX IF NOT EXISTS idx_industry_templates_active ON industry_templates(is_active);

COMMENT ON TABLE industry_templates IS '行業模板表：定義不同行業的默認配置';
COMMENT ON COLUMN industry_templates.code IS '行業代碼：retail, manufacturing, service, ecommerce, logistics, sme';
COMMENT ON COLUMN industry_templates.enabled_modules IS '啟用的模塊列表，JSON 數組格式';
COMMENT ON COLUMN industry_templates.default_fields IS '各表的默認動態字段配置，JSON 對象格式';

