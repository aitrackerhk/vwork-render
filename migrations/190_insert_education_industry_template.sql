-- 新增「教育/教學」行業模板
INSERT INTO industry_templates (code, name, name_en, description, description_en, enabled_modules, icon, sort_order, is_active)
VALUES (
    'education',
    '教育/教學',
    'Education',
    '適用於學校、補習班、培訓中心、線上課程等教育教學機構。內建課程管理、學員預約、導師管理等功能。',
    'Suitable for schools, tutoring centers, training institutions, and online course providers. Includes course management, student booking, instructor management and more.',
    '["courses", "service", "service_orders", "appointments", "customers", "users", "calendars", "reminders", "accounting", "hr"]'::jsonb,
    'bi-mortarboard',
    9,
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
