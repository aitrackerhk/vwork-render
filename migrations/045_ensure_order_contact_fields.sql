-- 確保訂單表有所有聯繫人字段

-- 添加 contact_name 字段（如果不存在）
ALTER TABLE orders 
ADD COLUMN IF NOT EXISTS contact_name VARCHAR(255);

-- 添加 contact_email 字段（如果不存在）
ALTER TABLE orders 
ADD COLUMN IF NOT EXISTS contact_email VARCHAR(255);

-- 添加 contact_phone 字段（如果不存在）
ALTER TABLE orders 
ADD COLUMN IF NOT EXISTS contact_phone VARCHAR(50);

-- 添加 contact_address 字段（如果不存在）
ALTER TABLE orders 
ADD COLUMN IF NOT EXISTS contact_address TEXT;

