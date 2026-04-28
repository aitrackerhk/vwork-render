-- 服務單重構：使其類似訂單結構
-- 1. 添加更多字段到 service_orders 表
-- 2. 添加 tenant_id 到 service_order_items 表
-- 3. 創建服務單標籤關聯表
-- 4. 在 appointments 表中添加 service_order_id 字段

-- 為 service_orders 表添加新字段
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'coupon_id') THEN
        ALTER TABLE service_orders ADD COLUMN coupon_id UUID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'points_used') THEN
        ALTER TABLE service_orders ADD COLUMN points_used INTEGER DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'points_earned') THEN
        ALTER TABLE service_orders ADD COLUMN points_earned INTEGER DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'points_discount') THEN
        ALTER TABLE service_orders ADD COLUMN points_discount DECIMAL(18,2) DEFAULT 0.00;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'coupon_discount') THEN
        ALTER TABLE service_orders ADD COLUMN coupon_discount DECIMAL(18,2) DEFAULT 0.00;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'referral_code') THEN
        ALTER TABLE service_orders ADD COLUMN referral_code VARCHAR(50);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'contact_name') THEN
        ALTER TABLE service_orders ADD COLUMN contact_name VARCHAR(255);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'contact_email') THEN
        ALTER TABLE service_orders ADD COLUMN contact_email VARCHAR(255);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'contact_phone') THEN
        ALTER TABLE service_orders ADD COLUMN contact_phone VARCHAR(50);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'contact_address') THEN
        ALTER TABLE service_orders ADD COLUMN contact_address TEXT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'salesperson_id') THEN
        ALTER TABLE service_orders ADD COLUMN salesperson_id UUID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'commission_amount') THEN
        ALTER TABLE service_orders ADD COLUMN commission_amount DECIMAL(15,2) DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'created_by') THEN
        ALTER TABLE service_orders ADD COLUMN created_by UUID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'updated_by') THEN
        ALTER TABLE service_orders ADD COLUMN updated_by UUID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'status') THEN
        ALTER TABLE service_orders ADD COLUMN status VARCHAR(50) DEFAULT 'draft';
    END IF;
END $$;

-- 修改現有字段（刪除 NOT NULL 約束，如果存在）
DO $$ 
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'customer_id' AND is_nullable = 'NO') THEN
        ALTER TABLE service_orders ALTER COLUMN customer_id DROP NOT NULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'order_number' AND is_nullable = 'NO') THEN
        ALTER TABLE service_orders ALTER COLUMN order_number DROP NOT NULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_orders' AND column_name = 'service_date' AND is_nullable = 'NO') THEN
        ALTER TABLE service_orders ALTER COLUMN service_date DROP NOT NULL;
    END IF;
END $$;

-- 為 service_order_items 表添加 tenant_id
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'service_order_items' AND column_name = 'tenant_id') THEN
        ALTER TABLE service_order_items ADD COLUMN tenant_id UUID;
        UPDATE service_order_items soi SET tenant_id = so.tenant_id FROM service_orders so WHERE soi.service_order_id = so.id AND soi.tenant_id IS NULL;
        ALTER TABLE service_order_items ALTER COLUMN tenant_id SET NOT NULL;
        CREATE INDEX idx_service_order_items_tenant_id ON service_order_items(tenant_id);
    END IF;
END $$;

-- 創建服務單標籤關聯表
CREATE TABLE IF NOT EXISTS service_order_label_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_order_id UUID NOT NULL REFERENCES service_orders(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES order_labels(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(service_order_id, label_id)
);

CREATE INDEX IF NOT EXISTS idx_service_order_label_relations_service_order_id ON service_order_label_relations(service_order_id);
CREATE INDEX IF NOT EXISTS idx_service_order_label_relations_label_id ON service_order_label_relations(label_id);

-- 在 appointments 表中添加 service_order_id 字段
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'appointments' AND column_name = 'service_order_id') THEN
        ALTER TABLE appointments ADD COLUMN service_order_id UUID REFERENCES service_orders(id) ON DELETE SET NULL;
        CREATE INDEX idx_appointments_service_order_id ON appointments(service_order_id);
    END IF;
END $$;

-- 添加外鍵約束
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_service_orders_coupon') THEN
        ALTER TABLE service_orders ADD CONSTRAINT fk_service_orders_coupon FOREIGN KEY (coupon_id) REFERENCES coupons(id) ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_service_orders_salesperson') THEN
        ALTER TABLE service_orders ADD CONSTRAINT fk_service_orders_salesperson FOREIGN KEY (salesperson_id) REFERENCES users(id) ON DELETE SET NULL;
    END IF;
END $$;
