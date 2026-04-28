-- 允許 users 表的 tenant_id 為 NULL（用於註冊時尚未創建租戶的情況）
ALTER TABLE users
    ALTER COLUMN tenant_id DROP NOT NULL;

