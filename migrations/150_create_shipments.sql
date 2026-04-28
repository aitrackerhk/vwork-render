-- 150_create_shipments.sql
-- 創建配送表

-- 配送記錄表
CREATE TABLE IF NOT EXISTS shipments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    shipment_number VARCHAR(50) UNIQUE NOT NULL,
    logistics_company_id UUID REFERENCES logistics_companies(id) ON DELETE SET NULL,
    tracking_number VARCHAR(100),
    order_id UUID REFERENCES orders(id) ON DELETE SET NULL,
    inventory_movement_id UUID, -- 參考出入貨記錄 ID（動態計算的數據，不建立外鍵）
    
    -- 發件人信息
    sender_name VARCHAR(100),
    sender_phone VARCHAR(50),
    sender_address TEXT,
    
    -- 收件人信息
    recipient_name VARCHAR(100),
    recipient_phone VARCHAR(50),
    recipient_address TEXT,
    
    -- 配送詳情
    weight DECIMAL(15, 3) DEFAULT 0,
    dimensions VARCHAR(100),
    item_count INTEGER DEFAULT 1,
    description TEXT,
    
    -- 費用
    shipping_fee DECIMAL(15, 2) DEFAULT 0,
    insurance_fee DECIMAL(15, 2) DEFAULT 0,
    total_fee DECIMAL(15, 2) DEFAULT 0,
    
    -- 狀態
    status VARCHAR(50) DEFAULT 'pending',
    estimated_delivery_at TIMESTAMP WITH TIME ZONE,
    actual_delivery_at TIMESTAMP WITH TIME ZONE,
    picked_up_at TIMESTAMP WITH TIME ZONE,
    
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 配送狀態歷史表
CREATE TABLE IF NOT EXISTS shipment_status_histories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id UUID NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL,
    location VARCHAR(255),
    description TEXT,
    occurred_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_shipments_tenant_id ON shipments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_shipments_status ON shipments(status);
CREATE INDEX IF NOT EXISTS idx_shipments_order_id ON shipments(order_id);
CREATE INDEX IF NOT EXISTS idx_shipments_inventory_movement_id ON shipments(inventory_movement_id);
CREATE INDEX IF NOT EXISTS idx_shipments_logistics_company_id ON shipments(logistics_company_id);
CREATE INDEX IF NOT EXISTS idx_shipments_tracking_number ON shipments(tracking_number);
CREATE INDEX IF NOT EXISTS idx_shipment_status_histories_shipment_id ON shipment_status_histories(shipment_id);

-- 觸發器：自動更新 updated_at
CREATE OR REPLACE FUNCTION update_shipments_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_shipments_updated_at ON shipments;
CREATE TRIGGER trigger_update_shipments_updated_at
    BEFORE UPDATE ON shipments
    FOR EACH ROW
    EXECUTE FUNCTION update_shipments_updated_at();
