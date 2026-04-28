-- 添加缺失的字段到 users 表

-- 添加 user_role 字段（用於向後兼容）
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS user_role VARCHAR(50) DEFAULT 'user';

-- 如果 role 字段存在，將數據複製到 user_role
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns 
               WHERE table_name = 'users' AND column_name = 'role') THEN
        UPDATE users SET user_role = role WHERE user_role IS NULL OR user_role = 'user';
    END IF;
END $$;

-- 添加 level_id 字段（用於向後兼容）
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS level_id UUID;

-- 添加 role_id 字段（如果不存在，已在 066_create_roles_table.sql 中添加）
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS role_id UUID REFERENCES roles(id) ON DELETE SET NULL;

-- 添加 department_id 字段
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS department_id UUID;

-- 添加外鍵約束（如果 departments 表存在）
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'departments') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints 
                       WHERE constraint_name = 'fk_users_department' AND table_name = 'users') THEN
            ALTER TABLE users 
            ADD CONSTRAINT fk_users_department 
            FOREIGN KEY (department_id) REFERENCES departments(id) ON DELETE SET NULL;
        END IF;
    END IF;
END $$;

-- 添加 salary 字段
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS salary DECIMAL(15,2) DEFAULT 0;

-- 添加 commission_rate 字段
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS commission_rate DECIMAL(5,2) DEFAULT 0;

-- 添加 profile_pic 字段
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS profile_pic VARCHAR(500);

-- 添加 salary_mode 字段
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS salary_mode VARCHAR(20) DEFAULT 'monthly';

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_users_user_role ON users(user_role);
CREATE INDEX IF NOT EXISTS idx_users_level_id ON users(level_id);
CREATE INDEX IF NOT EXISTS idx_users_department_id ON users(department_id);

-- 添加註釋
COMMENT ON COLUMN users.user_role IS '用戶角色（用於向後兼容）：admin, manager, user';
COMMENT ON COLUMN users.level_id IS '等級 ID（用於向後兼容）';
COMMENT ON COLUMN users.role_id IS '角色 ID（新角色系統）';
COMMENT ON COLUMN users.department_id IS '部門 ID';
COMMENT ON COLUMN users.salary IS '薪資';
COMMENT ON COLUMN users.commission_rate IS '佣金率（百分比）';
COMMENT ON COLUMN users.profile_pic IS '用戶頭像 URL';
COMMENT ON COLUMN users.salary_mode IS '薪資方式：monthly, hourly';

