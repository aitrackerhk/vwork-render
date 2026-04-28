-- Add source_type to orders: erp / pos / webstore
ALTER TABLE orders
ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) NOT NULL DEFAULT 'erp';

-- Backfill legacy rows
UPDATE orders
SET source_type = 'erp'
WHERE source_type IS NULL OR TRIM(source_type) = '';

CREATE INDEX IF NOT EXISTS idx_orders_source_type ON orders(source_type);


