-- Add default supplier and default warehouse to products
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS default_supplier_id UUID NULL,
    ADD COLUMN IF NOT EXISTS default_warehouse_id UUID NULL;

-- Indexes for faster lookups
CREATE INDEX IF NOT EXISTS idx_products_default_supplier_id ON products(default_supplier_id);
CREATE INDEX IF NOT EXISTS idx_products_default_warehouse_id ON products(default_warehouse_id);

