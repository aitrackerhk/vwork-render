-- Insert "不清楚" (Unknown) industry template
-- 插入"不清楚"行業模板，啟用所有模塊

INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'unknown',
    '不清楚',
    'Not Sure',
    '啟用所有功能模塊，讓您先試用所有功能，之後可以根據需要調整',
    'Enable all functional modules, try all features first, then adjust as needed',
    '["inventory", "pos", "customers", "orders", "products", "member", "coupons", "points", "service", "appointments", "service_orders", "users", "calendars", "reminders", "purchase", "suppliers", "warehouses", "warehouse", "logistics", "projects", "accounting", "invoices", "payments", "hr", "vehicles", "dining"]'::jsonb,
    'bi-question-circle',
    999,  -- 最大排序值，確保排在最底
    true
) ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    name_en = EXCLUDED.name_en,
    description = EXCLUDED.description,
    description_en = EXCLUDED.description_en,
    enabled_modules = EXCLUDED.enabled_modules,
    icon = EXCLUDED.icon,
    sort_order = EXCLUDED.sort_order,
    is_active = EXCLUDED.is_active,
    updated_at = NOW();

