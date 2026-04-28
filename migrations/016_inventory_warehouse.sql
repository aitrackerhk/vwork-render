-- 為庫存調整與盤點加入倉庫關聯

ALTER TABLE inventory_adjustments
    ADD COLUMN IF NOT EXISTS warehouse_id UUID REFERENCES warehouses(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_inventory_adjustments_warehouse
    ON inventory_adjustments(tenant_id, warehouse_id);

ALTER TABLE inventory_counts
    ADD COLUMN IF NOT EXISTS warehouse_id UUID REFERENCES warehouses(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_inventory_counts_warehouse
    ON inventory_counts(tenant_id, warehouse_id);

