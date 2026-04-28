-- 為會員等級表添加 is_default 字段（系統預設會員等級）

-- 添加 is_default 字段
ALTER TABLE member_levels 
ADD COLUMN IF NOT EXISTS is_default BOOLEAN DEFAULT false;

-- 創建索引以提高查詢性能
CREATE INDEX IF NOT EXISTS idx_member_levels_is_default ON member_levels(tenant_id, is_default);

