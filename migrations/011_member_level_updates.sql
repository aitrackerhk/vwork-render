-- 更新會員等級表，添加自動應用折扣和描述字段

-- 添加自動應用折扣字段
ALTER TABLE member_levels ADD COLUMN IF NOT EXISTS auto_apply_discount BOOLEAN DEFAULT false;

-- 添加描述字段
ALTER TABLE member_levels ADD COLUMN IF NOT EXISTS description TEXT;

