-- 添加客戶姓氏字段
ALTER TABLE customers ADD COLUMN IF NOT EXISTS last_name VARCHAR(100);

