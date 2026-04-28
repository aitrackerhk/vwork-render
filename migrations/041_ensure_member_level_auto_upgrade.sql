-- 確保 member_levels 表有 auto_upgrade 字段
ALTER TABLE member_levels
ADD COLUMN IF NOT EXISTS auto_upgrade BOOLEAN DEFAULT false;

