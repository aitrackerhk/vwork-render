-- Migration: 180_backfill_employee_numbers.sql
-- Description: 為現有沒有員工編號的用戶補上編號
-- 格式：EMP-YYYYMMDD-001（按租戶分組，按創建時間排序）

-- 使用 PL/pgSQL 為每個租戶的用戶生成員工編號
DO $$
DECLARE
    user_rec RECORD;
    current_tenant_id UUID := NULL;
    emp_count INT := 0;
    today_str TEXT := TO_CHAR(NOW(), 'YYYYMMDD');
    new_emp_number TEXT;
BEGIN
    -- 遍歷所有沒有員工編號的用戶，按租戶和創建時間排序
    FOR user_rec IN 
        SELECT id, tenant_id, created_at 
        FROM users 
        WHERE (employee_number IS NULL OR employee_number = '') 
          AND tenant_id IS NOT NULL
        ORDER BY tenant_id, created_at
    LOOP
        -- 如果是新租戶，重置計數器
        IF current_tenant_id IS NULL OR current_tenant_id != user_rec.tenant_id THEN
            current_tenant_id := user_rec.tenant_id;
            -- 查詢該租戶今天已有的員工編號數量
            SELECT COUNT(*) INTO emp_count
            FROM users 
            WHERE tenant_id = current_tenant_id 
              AND employee_number LIKE 'EMP-' || today_str || '-%';
        END IF;
        
        -- 遞增計數器並生成編號
        emp_count := emp_count + 1;
        new_emp_number := 'EMP-' || today_str || '-' || LPAD(emp_count::TEXT, 3, '0');
        
        -- 確保唯一性
        WHILE EXISTS (
            SELECT 1 FROM users 
            WHERE tenant_id = current_tenant_id 
              AND employee_number = new_emp_number
        ) LOOP
            emp_count := emp_count + 1;
            new_emp_number := 'EMP-' || today_str || '-' || LPAD(emp_count::TEXT, 3, '0');
        END LOOP;
        
        -- 更新用戶的員工編號
        UPDATE users SET employee_number = new_emp_number WHERE id = user_rec.id;
        
        RAISE NOTICE 'Updated user % with employee_number %', user_rec.id, new_emp_number;
    END LOOP;
END $$;

-- 顯示更新結果
SELECT id, tenant_id, name, employee_number, created_at 
FROM users 
WHERE employee_number LIKE 'EMP-%'
ORDER BY tenant_id, created_at
LIMIT 20;
