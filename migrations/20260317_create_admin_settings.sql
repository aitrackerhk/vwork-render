-- Admin Settings: platform-level key-value settings managed by vworkadmin
-- Used for storing admin notification emails and other global configuration
CREATE TABLE IF NOT EXISTS admin_settings (
    id          SERIAL PRIMARY KEY,
    key         VARCHAR(100) NOT NULL UNIQUE,
    value       TEXT NOT NULL DEFAULT '',
    description VARCHAR(255) DEFAULT '',
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Seed default admin_emails setting (empty by default, configured via vworkadmin)
INSERT INTO admin_settings (key, value, description)
VALUES ('admin_emails', '', '接收管理通知的 Email 地址（逗號分隔）')
ON CONFLICT (key) DO NOTHING;

-- Seed brevo_free_daily_limit setting
INSERT INTO admin_settings (key, value, description)
VALUES ('brevo_free_daily_limit', '300', 'Brevo 免費帳戶每日發送上限')
ON CONFLICT (key) DO NOTHING;
