-- 為 enterprises 表添加 timezone 字段，預設為香港時區
ALTER TABLE enterprises
ADD COLUMN IF NOT EXISTS timezone VARCHAR(50) DEFAULT 'Asia/Hong_Kong';

-- 更新現有記錄的 timezone 為預設值（如果為空）
UPDATE enterprises
SET timezone = 'Asia/Hong_Kong'
WHERE timezone IS NULL OR timezone = '';

