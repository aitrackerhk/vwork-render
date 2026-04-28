-- Insert default industry templates
-- 插入默認行業模板數據

-- 零售業模板
INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'retail',
    '零售業',
    'Retail',
    '門店管理、庫存追蹤、會員管理、銷售分析，一站式零售解決方案',
    'Store management, inventory tracking, membership management, sales analysis - one-stop retail solution',
    '["inventory", "pos", "customers", "orders", "products", "member", "coupons", "points", "accounting", "hr"]'::jsonb,
    'bi-shop',
    1,
    true
) ON CONFLICT (code) DO NOTHING;

-- 電商模板
INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'ecommerce',
    '電商',
    'E-commerce',
    '訂單處理、庫存管理、客戶服務、財務追蹤，電商運營全流程管理',
    'Order processing, inventory management, customer service, financial tracking - complete e-commerce operations management',
    '["inventory", "orders", "products", "customers", "invoices", "payments", "coupons", "points", "accounting", "hr"]'::jsonb,
    'bi-cart',
    2,
    true
) ON CONFLICT (code) DO NOTHING;

-- 服務業模板
INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'service',
    '服務業',
    'Service Industry',
    '預約管理、服務記錄、客戶追蹤、員工排班，服務業專屬管理系統',
    'Appointment management, service records, customer tracking, staff scheduling - dedicated service industry management system',
    '["service", "appointments", "service_orders", "customers", "users", "calendars", "reminders", "accounting", "hr"]'::jsonb,
    'bi-calendar-check',
    3,
    true
) ON CONFLICT (code) DO NOTHING;

-- 製造業模板
INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'manufacturing',
    '製造業',
    'Manufacturing',
    '生產管理、供應鏈追蹤、品質控制、成本分析，製造業完整解決方案',
    'Production management, supply chain tracking, quality control, cost analysis - complete manufacturing solution',
    '["inventory", "purchase", "suppliers", "products", "warehouses", "projects", "accounting", "hr"]'::jsonb,
    'bi-gear',
    4,
    true
) ON CONFLICT (code) DO NOTHING;

-- 物流業模板
INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'logistics',
    '物流業',
    'Logistics',
    '運輸管理、倉儲追蹤、配送優化、客戶服務，物流業高效管理工具',
    'Transportation management, warehouse tracking, delivery optimization, customer service - efficient logistics management tool',
    '["inventory", "warehouses", "orders", "customers", "vehicles", "projects", "accounting", "hr"]'::jsonb,
    'bi-truck',
    5,
    true
) ON CONFLICT (code) DO NOTHING;

-- 中小企業模板（全功能）
INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'sme',
    '中小企業',
    'SME',
    '靈活配置、快速上手、成本效益高，專為中小企業設計的管理系統',
    'Flexible configuration, quick start, cost-effective - management system designed specifically for small and medium enterprises',
    '["inventory", "orders", "products", "customers", "invoices", "payments", "accounting", "hr", "projects"]'::jsonb,
    'bi-building',
    6,
    true
) ON CONFLICT (code) DO NOTHING;

