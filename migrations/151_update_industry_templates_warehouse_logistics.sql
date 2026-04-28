-- Ensure warehouse/logistics modules exist in default industry templates
-- 補齊預設行業模板的倉庫/物流模組代碼，與新菜單結構對齊

WITH target AS (
    SELECT
        id,
        (
            SELECT to_jsonb(array_agg(DISTINCT elem))
            FROM (
                SELECT jsonb_array_elements_text(COALESCE(enabled_modules, '[]'::jsonb)) AS elem
                UNION ALL SELECT 'warehouse'
                UNION ALL SELECT 'logistics'
            ) AS s
        ) AS modules
    FROM industry_templates
    WHERE code IN ('retail', 'ecommerce', 'manufacturing', 'logistics', 'sme')
)
UPDATE industry_templates it
SET enabled_modules = target.modules,
    updated_at = NOW()
FROM target
WHERE it.id = target.id;
