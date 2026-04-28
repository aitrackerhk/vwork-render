-- Add dining module to unknown industry template if missing

UPDATE industry_templates
SET enabled_modules = (
    SELECT jsonb_agg(DISTINCT val ORDER BY val)
    FROM (
        SELECT jsonb_array_elements_text(COALESCE(industry_templates.enabled_modules, '[]'::jsonb)) AS val
        UNION ALL SELECT 'dining'
    ) s
)
WHERE code = 'unknown'
  AND NOT EXISTS (
      SELECT 1
      FROM jsonb_array_elements_text(COALESCE(industry_templates.enabled_modules, '[]'::jsonb)) v
      WHERE v = 'dining'
  );
