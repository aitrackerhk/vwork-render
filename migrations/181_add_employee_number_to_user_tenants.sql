-- Migration: 181_add_employee_number_to_user_tenants.sql
-- Description: 將員工編號移到 user_tenants 表，支持每個租戶獨立編號

-- 1. 添加 employee_number 字段到 user_tenants 表
ALTER TABLE user_tenants 
ADD COLUMN IF NOT EXISTS employee_number VARCHAR(100);

-- 2. 創建索引確保每個租戶內員工編號唯一
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_tenants_tenant_employee_number 
ON user_tenants(tenant_id, employee_number) 
WHERE employee_number IS NOT NULL AND employee_number != '';

-- 3. 遷移現有數據：從 users 表複製員工編號到 user_tenants
UPDATE user_tenants ut
SET employee_number = u.employee_number
FROM users u
WHERE ut.user_id = u.id 
  AND ut.tenant_id = u.tenant_id
  AND u.employee_number IS NOT NULL 
  AND u.employee_number != ''
  AND (ut.employee_number IS NULL OR ut.employee_number = '');

-- 4. 為沒有 user_tenants 記錄但有 tenant_id 的用戶創建關聯
INSERT INTO user_tenants (id, user_id, tenant_id, employee_number, is_default, created_at, updated_at)
SELECT 
    gen_random_uuid(),
    u.id,
    u.tenant_id,
    u.employee_number,
    true,
    COALESCE(u.created_at, NOW()),
    NOW()
FROM users u
WHERE u.tenant_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM user_tenants ut 
    WHERE ut.user_id = u.id AND ut.tenant_id = u.tenant_id
  );

-- 5. 為 user_tenants 中沒有員工編號的記錄生成編號
DO $$
DECLARE
    ut_rec RECORD;
    current_tenant_id UUID := NULL;
    emp_count INT := 0;
    today_str TEXT := TO_CHAR(NOW(), 'YYYYMMDD');
    new_emp_number TEXT;
BEGIN
    -- 遍歷所有沒有員工編號的 user_tenants 記錄
    FOR ut_rec IN 
        SELECT ut.id, ut.user_id, ut.tenant_id, ut.created_at 
        FROM user_tenants ut
        WHERE (ut.employee_number IS NULL OR ut.employee_number = '')
        ORDER BY ut.tenant_id, ut.created_at
    LOOP
        -- 如果是新租戶，重置計數器
        IF current_tenant_id IS NULL OR current_tenant_id != ut_rec.tenant_id THEN
            current_tenant_id := ut_rec.tenant_id;
            -- 查詢該租戶今天已有的員工編號數量
            SELECT COUNT(*) INTO emp_count
            FROM user_tenants 
            WHERE tenant_id = current_tenant_id 
              AND employee_number LIKE 'EMP-' || today_str || '-%';
        END IF;
        
        -- 遞增計數器並生成編號
        emp_count := emp_count + 1;
        new_emp_number := 'EMP-' || today_str || '-' || LPAD(emp_count::TEXT, 3, '0');
        
        -- 確保唯一性
        WHILE EXISTS (
            SELECT 1 FROM user_tenants 
            WHERE tenant_id = current_tenant_id 
              AND employee_number = new_emp_number
        ) LOOP
            emp_count := emp_count + 1;
            new_emp_number := 'EMP-' || today_str || '-' || LPAD(emp_count::TEXT, 3, '0');
        END LOOP;
        
        -- 更新員工編號
        UPDATE user_tenants SET employee_number = new_emp_number WHERE id = ut_rec.id;
        
        RAISE NOTICE 'Updated user_tenant % (user: %, tenant: %) with employee_number %', 
            ut_rec.id, ut_rec.user_id, ut_rec.tenant_id, new_emp_number;
    END LOOP;
END $$;

-- 顯示結果
SELECT ut.id, ut.user_id, ut.tenant_id, ut.employee_number, u.name, t.name as tenant_name
FROM user_tenants ut
JOIN users u ON ut.user_id = u.id
JOIN tenants t ON ut.tenant_id = t.id
ORDER BY ut.tenant_id, ut.employee_number
LIMIT 20;
