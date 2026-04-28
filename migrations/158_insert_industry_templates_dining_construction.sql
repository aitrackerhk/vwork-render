-- Insert additional industry templates: Dining, Construction

-- 餐飲模板
INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'dining',
    '餐飲',
    'Dining',
    '桌區/餐桌、點餐與收銀、菜單與庫存，適用於餐廳與飲品店等餐飲業態',
    'Tables/areas, ordering & POS, menu and inventory - for restaurants and beverage shops',
    '["dining", "pos", "orders", "products", "customers", "inventory", "accounting", "hr"]'::jsonb,
    'bi-cup-hot',
    7,
    true
) ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    name_en = EXCLUDED.name_en,
    description = EXCLUDED.description,
    description_en = EXCLUDED.description_en,
    enabled_modules = EXCLUDED.enabled_modules,
    icon = EXCLUDED.icon,
    sort_order = EXCLUDED.sort_order,
    is_active = EXCLUDED.is_active;

-- 建築模板
INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'construction',
    '建築',
    'Construction',
    '項目管理、採購與供應商、倉儲與成本追蹤，適用於工程/建築公司與承包商',
    'Projects, purchasing & suppliers, warehousing and cost tracking - for construction companies and contractors',
    '["projects", "purchase", "suppliers", "warehouses", "inventory", "accounting", "hr"]'::jsonb,
    'bi-bricks',
    8,
    true
) ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    name_en = EXCLUDED.name_en,
    description = EXCLUDED.description,
    description_en = EXCLUDED.description_en,
    enabled_modules = EXCLUDED.enabled_modules,
    icon = EXCLUDED.icon,
    sort_order = EXCLUDED.sort_order,
    is_active = EXCLUDED.is_active;
