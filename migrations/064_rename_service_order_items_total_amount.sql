-- 將 service_order_items 表的 total_amount 列重命名為 total_price
-- 以與 order_items 表保持一致，並匹配 Go 模型中的 TotalPrice 字段

DO $$ 
BEGIN
    -- 檢查列是否存在，如果存在則重命名
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'service_order_items' 
        AND column_name = 'total_amount'
    ) THEN
        ALTER TABLE service_order_items 
        RENAME COLUMN total_amount TO total_price;
    END IF;
END $$;

