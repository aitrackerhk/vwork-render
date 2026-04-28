-- 確保訂閱計劃數據存在且正確
-- 此遷移確保 subscription_plans 表有正確的數據，並添加唯一約束

-- 1. 為 name 字段添加唯一約束（如果不存在）
DO $$
BEGIN
    -- 先去重，避免唯一約束新增失敗
    IF EXISTS (SELECT 1 FROM subscription_plans) THEN
        WITH ranked AS (
            SELECT ctid, name,
                   ROW_NUMBER() OVER (PARTITION BY name ORDER BY created_at ASC NULLS LAST) AS rn
            FROM subscription_plans
        )
        DELETE FROM subscription_plans
        WHERE ctid IN (SELECT ctid FROM ranked WHERE rn > 1);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'subscription_plans_name_key'
    ) THEN
        ALTER TABLE subscription_plans ADD CONSTRAINT subscription_plans_name_key UNIQUE (name);
    END IF;
END $$;

-- 2. 使用 INSERT ... ON CONFLICT 確保數據存在且正確
-- 如果記錄已存在，則更新它；如果不存在，則插入新記錄
INSERT INTO subscription_plans (name, display_name, price, yearly_price, interval, is_active, stripe_price_id)
VALUES
    ('monthly', '月付方案', 380.00, NULL, 'month', true, NULL),
    ('yearly', '年付方案', 250.00, 3000.00, 'year', true, NULL)
ON CONFLICT (name) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    price = EXCLUDED.price,
    yearly_price = EXCLUDED.yearly_price,
    interval = EXCLUDED.interval,
    is_active = true,  -- 確保啟用
    updated_at = CURRENT_TIMESTAMP;

-- 3. 確保所有訂閱計劃都是啟用狀態（防止之前被禁用）
UPDATE subscription_plans 
SET is_active = true, updated_at = CURRENT_TIMESTAMP
WHERE name IN ('monthly', 'yearly') AND is_active = false;

