-- 插入默認角色：管理員、經理、一般
-- 為每個租戶創建三個默認角色

DO $$
DECLARE
    tenant_record RECORD;
    admin_role_id UUID;
    manager_role_id UUID;
    user_role_id UUID;
BEGIN
    -- 為每個租戶創建默認角色
    FOR tenant_record IN SELECT id FROM tenants LOOP
        -- 檢查是否已經存在默認角色（避免重複插入）
        IF NOT EXISTS (
            SELECT 1 FROM roles 
            WHERE tenant_id = tenant_record.id 
            AND name IN ('管理員', '經理', '一般')
        ) THEN
            -- 創建管理員角色（擁有所有權限）
            INSERT INTO roles (id, tenant_id, name, description, permissions, status, created_at, updated_at)
            VALUES (
                gen_random_uuid(),
                tenant_record.id,
                '管理員',
                '擁有所有系統權限的管理員角色',
                '[]'::jsonb,
                'active',
                CURRENT_TIMESTAMP,
                CURRENT_TIMESTAMP
            )
            RETURNING id INTO admin_role_id;

            -- 創建經理角色
            INSERT INTO roles (id, tenant_id, name, description, permissions, status, created_at, updated_at)
            VALUES (
                gen_random_uuid(),
                tenant_record.id,
                '經理',
                '擁有大部分管理權限的經理角色',
                '[]'::jsonb,
                'active',
                CURRENT_TIMESTAMP,
                CURRENT_TIMESTAMP
            )
            RETURNING id INTO manager_role_id;

            -- 創建一般角色
            INSERT INTO roles (id, tenant_id, name, description, permissions, status, created_at, updated_at)
            VALUES (
                gen_random_uuid(),
                tenant_record.id,
                '一般',
                '擁有基本操作權限的一般用戶角色',
                '[]'::jsonb,
                'active',
                CURRENT_TIMESTAMP,
                CURRENT_TIMESTAMP
            )
            RETURNING id INTO user_role_id;
        END IF;
    END LOOP;
END $$;

