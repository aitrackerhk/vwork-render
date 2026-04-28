-- 為 service_orders 表補上 store_id（服務單所屬店舖）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'service_orders'
          AND column_name = 'store_id'
    ) THEN
        ALTER TABLE service_orders
            ADD COLUMN store_id UUID REFERENCES stores(id) ON DELETE SET NULL;
        CREATE INDEX IF NOT EXISTS idx_service_orders_store_id ON service_orders(store_id);
    END IF;
END $$;


