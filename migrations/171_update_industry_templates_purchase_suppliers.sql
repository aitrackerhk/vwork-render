-- Ensure all industry templates include purchase and suppliers modules

UPDATE industry_templates
SET enabled_modules = (
    SELECT jsonb_agg(to_jsonb(val))
    FROM (
        SELECT jsonb_array_elements_text(COALESCE(industry_templates.enabled_modules, '[]'::jsonb)) AS val
        UNION
        SELECT 'purchase'
        UNION
        SELECT 'suppliers'
    ) s
)
WHERE is_active = true;
