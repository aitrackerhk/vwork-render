-- 為 warehouses 和 logistics_companies 表添加 is_default 字段（系統預設）

-- 添加 is_default 字段到 warehouses
ALTER TABLE warehouses 
ADD COLUMN IF NOT EXISTS is_default BOOLEAN DEFAULT false;

-- 創建索引以提高查詢性能
CREATE INDEX IF NOT EXISTS idx_warehouses_is_default ON warehouses(tenant_id, is_default);

-- 添加 is_default 字段到 logistics_companies
ALTER TABLE logistics_companies 
ADD COLUMN IF NOT EXISTS is_default BOOLEAN DEFAULT false;

-- 創建索引以提高查詢性能
CREATE INDEX IF NOT EXISTS idx_logistics_companies_is_default ON logistics_companies(tenant_id, is_default);

