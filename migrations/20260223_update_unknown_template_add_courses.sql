-- Add "courses" module to the "不清楚" (unknown) industry template
-- and backfill existing tenants using unknown template

-- 1. Update the unknown template's enabled_modules to include "courses"
UPDATE industry_templates
SET enabled_modules = enabled_modules || '["courses"]'::jsonb,
    updated_at = NOW()
WHERE code = 'unknown'
  AND NOT (enabled_modules @> '"courses"'::jsonb);

-- 2. Backfill: enable "courses" module for existing tenants using unknown template
INSERT INTO tenant_modules (tenant_id, module_code, is_enabled)
SELECT t.id, 'courses', true
FROM tenants t
WHERE t.industry_template_id IN (SELECT id FROM industry_templates WHERE code = 'unknown')
   OR (t.extra_fields->>'industry_template_unknown')::boolean = true
ON CONFLICT (tenant_id, module_code) DO UPDATE SET is_enabled = true;

-- 3. Backfill: create "課程" service type for existing tenants using unknown template (if not exists)
INSERT INTO service_types (tenant_id, name, code, description, status)
SELECT t.id, '課程', 'course', '課程教學類服務', 'active'
FROM tenants t
WHERE (t.industry_template_id IN (SELECT id FROM industry_templates WHERE code = 'unknown')
       OR (t.extra_fields->>'industry_template_unknown')::boolean = true)
  AND NOT EXISTS (
      SELECT 1 FROM service_types st WHERE st.tenant_id = t.id AND st.code = 'course'
  );
