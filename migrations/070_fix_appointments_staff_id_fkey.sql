-- 修改 appointments 表的 staff_id 外鍵，從 users 改為 service_staff
-- 首先刪除舊的外鍵約束
ALTER TABLE appointments 
    DROP CONSTRAINT IF EXISTS appointments_staff_id_fkey;

-- 添加新的外鍵約束，指向 service_staff 表
ALTER TABLE appointments 
    ADD CONSTRAINT appointments_staff_id_fkey 
    FOREIGN KEY (staff_id) 
    REFERENCES service_staff(id) 
    ON DELETE SET NULL;

