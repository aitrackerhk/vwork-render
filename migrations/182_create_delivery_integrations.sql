-- 外賣平台整合設定表
-- 支持 Foodpanda, Keeta, Deliveroo 等外賣平台
CREATE TABLE IF NOT EXISTS delivery_integrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    store_id UUID REFERENCES stores(id) ON DELETE SET NULL,
    
    -- 平台類型: foodpanda, keeta, deliveroo
    platform VARCHAR(50) NOT NULL,
    
    -- 平台商戶信息
    merchant_id VARCHAR(255),
    merchant_name VARCHAR(255),
    
    -- API 認證 (加密儲存)
    api_key TEXT,
    api_secret TEXT,
    access_token TEXT,
    refresh_token TEXT,
    token_expires_at TIMESTAMP,
    
    -- Webhook 設定
    webhook_secret VARCHAR(255),
    webhook_url VARCHAR(500),
    
    -- 狀態
    is_enabled BOOLEAN DEFAULT true,
    is_connected BOOLEAN DEFAULT false,
    last_sync_at TIMESTAMP,
    last_error TEXT,
    
    -- 設定選項 (JSONB 擴展)
    settings JSONB DEFAULT '{}',
    
    -- 審計字段
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    
    -- 唯一約束：每個租戶每個平台只能有一個整合
    UNIQUE(tenant_id, platform)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_delivery_integrations_tenant_id ON delivery_integrations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_delivery_integrations_platform ON delivery_integrations(platform);
CREATE INDEX IF NOT EXISTS idx_delivery_integrations_is_enabled ON delivery_integrations(is_enabled);

-- 外賣訂單表
CREATE TABLE IF NOT EXISTS delivery_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    store_id UUID REFERENCES stores(id) ON DELETE SET NULL,
    integration_id UUID REFERENCES delivery_integrations(id) ON DELETE SET NULL,
    
    -- 關聯到內部訂單
    order_id UUID REFERENCES orders(id) ON DELETE SET NULL,
    
    -- 平台訂單信息
    platform VARCHAR(50) NOT NULL,
    platform_order_id VARCHAR(255) NOT NULL,
    platform_order_number VARCHAR(100),
    
    -- 訂單狀態
    -- pending, confirmed, preparing, ready_for_pickup, picked_up, delivered, cancelled, failed
    status VARCHAR(50) DEFAULT 'pending',
    platform_status VARCHAR(100),
    
    -- 客戶信息
    customer_name VARCHAR(255),
    customer_phone VARCHAR(100),
    customer_address TEXT,
    customer_notes TEXT,
    
    -- 配送信息
    delivery_type VARCHAR(50), -- delivery, pickup, dine_in
    estimated_pickup_time TIMESTAMP,
    estimated_delivery_time TIMESTAMP,
    actual_pickup_time TIMESTAMP,
    actual_delivery_time TIMESTAMP,
    
    -- 騎手信息
    rider_name VARCHAR(255),
    rider_phone VARCHAR(100),
    rider_tracking_url TEXT,
    
    -- 金額信息
    subtotal DECIMAL(15, 2) DEFAULT 0,
    delivery_fee DECIMAL(15, 2) DEFAULT 0,
    platform_fee DECIMAL(15, 2) DEFAULT 0,
    discount_amount DECIMAL(15, 2) DEFAULT 0,
    total_amount DECIMAL(15, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'HKD',
    
    -- 訂單項目 (JSONB)
    items JSONB DEFAULT '[]',
    
    -- 原始數據
    raw_data JSONB DEFAULT '{}',
    
    -- 審計字段
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    confirmed_at TIMESTAMP,
    cancelled_at TIMESTAMP,
    cancel_reason TEXT,
    
    -- 唯一約束
    UNIQUE(tenant_id, platform, platform_order_id)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_delivery_orders_tenant_id ON delivery_orders(tenant_id);
CREATE INDEX IF NOT EXISTS idx_delivery_orders_platform ON delivery_orders(platform);
CREATE INDEX IF NOT EXISTS idx_delivery_orders_status ON delivery_orders(status);
CREATE INDEX IF NOT EXISTS idx_delivery_orders_order_id ON delivery_orders(order_id);
CREATE INDEX IF NOT EXISTS idx_delivery_orders_platform_order_id ON delivery_orders(platform_order_id);
CREATE INDEX IF NOT EXISTS idx_delivery_orders_created_at ON delivery_orders(created_at);

-- 外賣訂單狀態歷史
CREATE TABLE IF NOT EXISTS delivery_order_status_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_order_id UUID NOT NULL REFERENCES delivery_orders(id) ON DELETE CASCADE,
    
    status VARCHAR(50) NOT NULL,
    platform_status VARCHAR(100),
    notes TEXT,
    raw_event JSONB DEFAULT '{}',
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_delivery_order_status_history_order_id ON delivery_order_status_history(delivery_order_id);
CREATE INDEX IF NOT EXISTS idx_delivery_order_status_history_created_at ON delivery_order_status_history(created_at);

-- 添加餐飲設定表的外賣整合欄位
ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS category VARCHAR(100);

COMMENT ON TABLE delivery_integrations IS '外賣平台整合設定（Foodpanda, Keeta, Deliveroo）';
COMMENT ON TABLE delivery_orders IS '從外賣平台同步的訂單';
COMMENT ON TABLE delivery_order_status_history IS '外賣訂單狀態變更歷史';
