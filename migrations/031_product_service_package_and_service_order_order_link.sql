-- 添加產品服務套票字段和服務單關聯訂單字段

-- 為 products 表添加服務套票相關字段
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'products' AND column_name = 'is_service_package') THEN
        ALTER TABLE products ADD COLUMN is_service_package BOOLEAN DEFAULT false;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'products' AND column_name = 'service_package_service_id') THEN
        ALTER TABLE products ADD COLUMN service_package_service_id UUID REFERENCES services(id) ON DELETE SET NULL;
        CREATE INDEX idx_products_service_package_service_id ON products(service_package_service_id);
    END IF;
END $$;

-- 為 service_orders 表添加關聯訂單字段
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'order_id') THEN
        ALTER TABLE service_orders ADD COLUMN order_id UUID REFERENCES orders(id) ON DELETE SET NULL;
        CREATE INDEX idx_service_orders_order_id ON service_orders(order_id);
    END IF;
END $$;

