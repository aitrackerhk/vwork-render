-- Create system_settings table
-- 系統設定表：存儲系統級別的設定（如 AI prompt）

CREATE TABLE IF NOT EXISTS system_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(255) NOT NULL UNIQUE,
    value TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_settings_key ON system_settings(key);

-- 插入默認的 AI prompt 設定
INSERT INTO system_settings (key, value, description)
VALUES (
    'ai_system_prompt',
    '你是 Vai 助手，專門處理業務相關的事務。你只回答與業務、工作、商業相關的問題，不處理私人事務。如果用戶詢問私人問題，請禮貌地告知你只處理業務相關的事務。',
    'AI 助手的系統提示詞'
)
ON CONFLICT (key) DO NOTHING;

COMMENT ON TABLE system_settings IS '系統設定表：存儲系統級別的設定';
COMMENT ON COLUMN system_settings.key IS '設定鍵（唯一）';
COMMENT ON COLUMN system_settings.value IS '設定值';
COMMENT ON COLUMN system_settings.description IS '設定說明';

