-- 為 service_staff 表添加 name 和 phone 欄位
-- 這些欄位允許服務員不關聯用戶時也能獨立存在

ALTER TABLE service_staff
    ADD COLUMN IF NOT EXISTS name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS phone VARCHAR(50);

-- 如果 name 欄位已存在但允許 NULL，改為 NOT NULL（但需要先處理現有 NULL 值）
-- 注意：如果表中已有數據且 name 為 NULL，需要先更新這些記錄
DO $$
BEGIN
    -- 更新現有的 NULL name 值（如果有的話）
    UPDATE service_staff 
    SET name = COALESCE(
        (SELECT name FROM users WHERE users.id = service_staff.user_id),
        '未命名服務員'
    )
    WHERE name IS NULL;
    
    -- 更新現有的 NULL phone 值（如果有的話）
    UPDATE service_staff 
    SET phone = COALESCE(
        (SELECT phone FROM users WHERE users.id = service_staff.user_id),
        ''
    )
    WHERE phone IS NULL;
END $$;

-- 設置 name 為 NOT NULL（在更新完所有 NULL 值後）
ALTER TABLE service_staff
    ALTER COLUMN name SET NOT NULL;

-- 添加註釋
COMMENT ON COLUMN service_staff.name IS '服務員姓名（可獨立於用戶表）';
COMMENT ON COLUMN service_staff.phone IS '服務員電話（可獨立於用戶表）';

