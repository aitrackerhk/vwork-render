-- 會員等級自動升級功能更新
-- 1. 添加最低購物金額字段
-- 2. 將 auto_apply_discount 改為 auto_upgrade
-- 3. 確保 level_order 字段存在

-- 添加 min_purchase_amount 字段（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = current_schema()
        AND table_name = 'member_levels' 
        AND column_name = 'min_purchase_amount'
    ) THEN
        ALTER TABLE member_levels ADD COLUMN min_purchase_amount DECIMAL(10,2) DEFAULT 0.00;
    END IF;
END $$;

-- 將 auto_apply_discount 重命名為 auto_upgrade（如果存在）
DO $$ 
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = current_schema()
        AND table_name = 'member_levels' 
        AND column_name = 'auto_apply_discount'
    )
    AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
        AND table_name = 'member_levels'
        AND column_name = 'auto_upgrade'
    ) THEN
        ALTER TABLE member_levels RENAME COLUMN auto_apply_discount TO auto_upgrade;
    END IF;
END $$;

-- 如果 auto_upgrade 不存在，添加它
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = current_schema()
        AND table_name = 'member_levels' 
        AND column_name = 'auto_upgrade'
    ) THEN
        ALTER TABLE member_levels ADD COLUMN auto_upgrade BOOLEAN DEFAULT false;
    END IF;
END $$;

-- 確保 level_order 字段存在
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = current_schema()
        AND table_name = 'member_levels' 
        AND column_name = 'level_order'
    ) THEN
        ALTER TABLE member_levels ADD COLUMN level_order INTEGER DEFAULT 0;
    END IF;
END $$;

