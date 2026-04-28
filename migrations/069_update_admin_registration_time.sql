-- 更新 admin@test.com 的註冊時間為現在
UPDATE users
SET created_at = CURRENT_TIMESTAMP
WHERE email = 'admin@test.com';

-- 確保 trial_expires_at 列存在
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name = 'tenants' AND column_name = 'trial_expires_at') THEN
        ALTER TABLE tenants ADD COLUMN trial_expires_at TIMESTAMP WITH TIME ZONE;
    END IF;
END $$;

-- 同時更新對應租戶的 trial_expires_at（如果存在）
-- 假設試用期為 3 天，從現在開始計算
UPDATE tenants t
SET trial_expires_at = CURRENT_TIMESTAMP + INTERVAL '3 days',
    plan = 'trial',
    status = 'active'
WHERE t.id IN (
    SELECT tenant_id FROM users WHERE email = 'admin@test.com'
)
AND (trial_expires_at IS NULL OR trial_expires_at < CURRENT_TIMESTAMP);

