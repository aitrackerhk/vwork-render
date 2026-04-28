-- 151_update_unknown_template_tenants.sql
-- 為所有使用 unknown 模板的 tenants 添加 warehouse 和 logistics 模組

-- 為使用 unknown 模板的所有 tenants 添加 warehouse 模組
INSERT INTO tenant_modules (tenant_id, module_code, is_enabled, config)
SELECT t.id, 'warehouse', true, '{}'::jsonb
FROM tenants t
JOIN industry_templates it ON t.industry_template_id = it.id
WHERE it.code = 'unknown'
ON CONFLICT (tenant_id, module_code) DO UPDATE SET is_enabled = true, updated_at = NOW();

-- 為使用 unknown 模板的所有 tenants 添加 logistics 模組
INSERT INTO tenant_modules (tenant_id, module_code, is_enabled, config)
SELECT t.id, 'logistics', true, '{}'::jsonb
FROM tenants t
JOIN industry_templates it ON t.industry_template_id = it.id
WHERE it.code = 'unknown'
ON CONFLICT (tenant_id, module_code) DO UPDATE SET is_enabled = true, updated_at = NOW();
