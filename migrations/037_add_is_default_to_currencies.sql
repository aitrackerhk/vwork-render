-- 為貨幣表添加 is_default 字段（系統預設）

-- 添加 is_default 字段
ALTER TABLE currencies 
ADD COLUMN IF NOT EXISTS is_default BOOLEAN DEFAULT false;

-- 創建索引以提高查詢性能
CREATE INDEX IF NOT EXISTS idx_currencies_is_default ON currencies(is_default);

-- 將 HKD 設置為默認貨幣
UPDATE currencies 
SET is_default = true 
WHERE code = 'HKD';

-- 確保只有一個默認貨幣（如果有多個，只保留 HKD）
DO $$
DECLARE
    default_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO default_count FROM currencies WHERE is_default = true;
    IF default_count > 1 THEN
        -- 如果有多個默認貨幣，只保留 HKD，其他設為 false
        UPDATE currencies 
        SET is_default = false 
        WHERE code != 'HKD' AND is_default = true;
    END IF;
END $$;

