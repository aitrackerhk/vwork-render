-- 為 suppliers 表添加 last_name 欄位
ALTER TABLE suppliers
    ADD COLUMN IF NOT EXISTS last_name VARCHAR(255);

-- 添加註釋
COMMENT ON COLUMN suppliers.last_name IS '姓氏（可選）';

