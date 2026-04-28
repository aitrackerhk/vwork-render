-- 164_inventory_settings_item_by_item.sql
-- 添加續件出入庫設定欄位

-- 添加續件出庫欄位
ALTER TABLE inventory_settings ADD COLUMN IF NOT EXISTS requires_item_by_item_outbound BOOLEAN DEFAULT false;

-- 添加續件入庫欄位
ALTER TABLE inventory_settings ADD COLUMN IF NOT EXISTS requires_item_by_item_inbound BOOLEAN DEFAULT false;

-- 添加註解
COMMENT ON COLUMN inventory_settings.requires_item_by_item_outbound IS '是否需要續件出庫（逐件掃碼）';
COMMENT ON COLUMN inventory_settings.requires_item_by_item_inbound IS '是否需要續件入庫（逐件掃碼）';
