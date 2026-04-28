-- 添加加盟商 AI Coins 配額欄位
ALTER TABLE sales_partner_applications 
ADD COLUMN IF NOT EXISTS monthly_ai_coins INT;

-- 添加註解
COMMENT ON COLUMN sales_partner_applications.monthly_ai_coins IS '加盟商客戶的 AI Coins 月配額 (500-650)，NULL = 使用預設 550';
