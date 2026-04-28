-- 添加每日免費次數欄位到 tenant_ai_coins 表
ALTER TABLE tenant_ai_coins 
ADD COLUMN IF NOT EXISTS daily_free_used INT DEFAULT 0,
ADD COLUMN IF NOT EXISTS daily_free_reset_at TIMESTAMP;

-- 初始化每日免費重置時間為明天凌晨
UPDATE tenant_ai_coins 
SET daily_free_reset_at = DATE_TRUNC('day', NOW()) + INTERVAL '1 day'
WHERE daily_free_reset_at IS NULL;

-- 更新免費用戶的月配額為 0（每日 5 次免費取代原來的 10 coins 月配額）
UPDATE tenant_ai_coins tac
SET monthly_allotment = 0
FROM tenants t
WHERE tac.tenant_id = t.id 
  AND (t.plan = 'free' OR t.plan = 'trial' OR t.plan IS NULL OR t.plan = '');

-- 添加 daily_free 交易類型的註解
COMMENT ON COLUMN tenant_ai_coins.daily_free_used IS '今日已用免費查詢次數';
COMMENT ON COLUMN tenant_ai_coins.daily_free_reset_at IS '每日免費次數重置時間';
