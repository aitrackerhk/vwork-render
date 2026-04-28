-- Create tenant modules table
-- 租戶模塊表：每個租戶的模塊開關配置

CREATE TABLE IF NOT EXISTS tenant_modules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    module_code VARCHAR(50) NOT NULL,  -- "inventory", "hr", "accounting", "pos", "service", "project"
    is_enabled BOOLEAN DEFAULT true,
    config JSONB DEFAULT '{}'::jsonb,  -- 模塊特定配置
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, module_code)
);

CREATE INDEX IF NOT EXISTS idx_tenant_modules_tenant_id ON tenant_modules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_modules_code ON tenant_modules(tenant_id, module_code);
CREATE INDEX IF NOT EXISTS idx_tenant_modules_enabled ON tenant_modules(tenant_id, is_enabled);

COMMENT ON TABLE tenant_modules IS '租戶模塊表：管理每個租戶啟用的模塊';
COMMENT ON COLUMN tenant_modules.module_code IS '模塊代碼：inventory, hr, accounting, pos, service, project 等';
COMMENT ON COLUMN tenant_modules.config IS '模塊特定配置，JSON 對象格式';

-- Add industry_template_id to tenants table
ALTER TABLE tenants 
ADD COLUMN IF NOT EXISTS industry_template_id UUID REFERENCES industry_templates(id);

CREATE INDEX IF NOT EXISTS idx_tenants_industry_template_id ON tenants(industry_template_id);

COMMENT ON COLUMN tenants.industry_template_id IS '關聯的行業模板 ID，可選';

