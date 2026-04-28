-- Fix order_number uniqueness scope:
-- Previously service_orders.order_number and purchase_orders.order_number were UNIQUE globally,
-- causing collisions across tenants. Make them unique per tenant instead.

-- purchase_orders
ALTER TABLE purchase_orders
DROP CONSTRAINT IF EXISTS purchase_orders_order_number_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_purchase_orders_tenant_order_number
ON purchase_orders (tenant_id, order_number);

-- service_orders
ALTER TABLE service_orders
DROP CONSTRAINT IF EXISTS service_orders_order_number_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_service_orders_tenant_order_number
ON service_orders (tenant_id, order_number);


