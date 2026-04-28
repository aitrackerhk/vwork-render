-- 服務員增加所屬服務類別欄位
ALTER TABLE service_staff ADD COLUMN IF NOT EXISTS service_type_id UUID REFERENCES service_types(id) ON DELETE SET NULL;

-- 建立索引加速查詢
CREATE INDEX IF NOT EXISTS idx_service_staff_service_type_id ON service_staff(service_type_id);
