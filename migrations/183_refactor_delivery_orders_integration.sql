-- Migration: 183_refactor_delivery_orders_integration.sql
-- Description: 重構外賣訂單架構 - 整合到現有 Order 系統
-- Created: 2026-02-05
--
-- 設計理念：
-- 1. 外賣訂單主體存入 orders 表（統一訂單管理）
-- 2. delivery_order_details 只存放外賣平台特有資訊（補充表）
-- 3. 通過 order.source_type = 'delivery' 區分外賣訂單
-- 4. 一對一關聯：orders.id = delivery_order_details.order_id

-- 1. 添加 orders 表的外賣相關欄位
ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivery_platform VARCHAR(50);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS platform_order_id VARCHAR(255);

-- 添加索引方便查詢
CREATE INDEX IF NOT EXISTS idx_orders_delivery_platform ON orders(delivery_platform) WHERE delivery_platform IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_orders_platform_order_id ON orders(platform_order_id) WHERE platform_order_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_platform_order_unique ON orders(tenant_id, delivery_platform, platform_order_id) WHERE platform_order_id IS NOT NULL;

-- 2. 重命名並簡化 delivery_orders 表為 delivery_order_details（補充資訊表）
-- 先備份原表
ALTER TABLE delivery_orders RENAME TO delivery_orders_backup;

-- 創建新的補充資訊表（只存放外賣平台特有資訊）
CREATE TABLE IF NOT EXISTS delivery_order_details (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    integration_id UUID REFERENCES delivery_integrations(id),
    
    -- 平台資訊（冗餘存放方便查詢）
    platform VARCHAR(50) NOT NULL,
    platform_order_id VARCHAR(255) NOT NULL,
    platform_order_number VARCHAR(100),
    platform_status VARCHAR(100),
    
    -- 配送資訊（Order 表沒有的）
    delivery_type VARCHAR(50) DEFAULT 'delivery', -- delivery, pickup, dine_in
    estimated_pickup_time TIMESTAMP WITH TIME ZONE,
    estimated_delivery_time TIMESTAMP WITH TIME ZONE,
    actual_pickup_time TIMESTAMP WITH TIME ZONE,
    actual_delivery_time TIMESTAMP WITH TIME ZONE,
    
    -- 騎手資訊
    rider_name VARCHAR(255),
    rider_phone VARCHAR(100),
    rider_tracking_url TEXT,
    
    -- 外賣平台費用明細
    platform_fee DECIMAL(15,2) DEFAULT 0,
    delivery_fee DECIMAL(15,2) DEFAULT 0,
    platform_discount DECIMAL(15,2) DEFAULT 0,
    
    -- 原始數據（完整保留平台返回的 JSON）
    raw_data JSONB DEFAULT '{}',
    
    -- 取消資訊
    cancelled_at TIMESTAMP WITH TIME ZONE,
    cancel_reason TEXT,
    cancelled_by VARCHAR(50), -- platform, merchant, customer
    
    -- 審計
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    confirmed_at TIMESTAMP WITH TIME ZONE,
    
    -- 唯一約束
    CONSTRAINT uq_delivery_order_details_order UNIQUE (order_id),
    CONSTRAINT uq_delivery_order_details_platform UNIQUE (platform, platform_order_id)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_delivery_order_details_order_id ON delivery_order_details(order_id);
CREATE INDEX IF NOT EXISTS idx_delivery_order_details_platform ON delivery_order_details(platform);
CREATE INDEX IF NOT EXISTS idx_delivery_order_details_integration ON delivery_order_details(integration_id);

-- 3. 將備份表的數據遷移到新架構（如果有數據的話）
-- 這個 migration 假設 delivery_orders 還沒有實際數據，如有需要可手動遷移

-- 4. 更新狀態歷史表的外鍵
-- 先刪除舊的外鍵約束（如果存在）
ALTER TABLE delivery_order_status_history DROP CONSTRAINT IF EXISTS delivery_order_status_history_delivery_order_id_fkey;

-- 添加新的外鍵（關聯到 orders 表）
ALTER TABLE delivery_order_status_history RENAME COLUMN delivery_order_id TO order_id;
ALTER TABLE delivery_order_status_history ADD CONSTRAINT delivery_order_status_history_order_id_fkey 
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE;

-- 5. 擴展 order_items 表支持外賣訂單項目（product_id 可為空）
-- 添加平台商品資訊欄位
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS platform_item_id VARCHAR(255);
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS item_name VARCHAR(500);
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS item_options JSONB DEFAULT '[]';

-- 確保 product_id 允許為空（外賣訂單可能沒有對應的 vWork 產品）
ALTER TABLE order_items ALTER COLUMN product_id DROP NOT NULL;

COMMENT ON COLUMN order_items.platform_item_id IS '外賣平台商品ID';
COMMENT ON COLUMN order_items.item_name IS '商品名稱（外賣訂單直接存名稱，不依賴 products 表）';
COMMENT ON COLUMN order_items.item_options IS '商品選項/加料 JSON（如：[{"name":"少冰","price":0},{"name":"加珍珠","price":5}]）';

-- 6. 創建產品映射表（可選功能：將外賣平台商品映射到 vWork 產品）
CREATE TABLE IF NOT EXISTS delivery_product_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    integration_id UUID REFERENCES delivery_integrations(id) ON DELETE CASCADE,
    
    -- 平台商品資訊
    platform VARCHAR(50) NOT NULL,
    platform_item_id VARCHAR(255) NOT NULL,
    platform_item_name VARCHAR(500),
    platform_category VARCHAR(255),
    
    -- 映射到 vWork 產品（可為空 = 不映射）
    product_id UUID REFERENCES products(id) ON DELETE SET NULL,
    
    -- 價格差異（外賣平台價格可能不同）
    platform_price DECIMAL(15,2),
    price_difference DECIMAL(15,2) DEFAULT 0, -- 與 vWork 產品的價差
    
    -- 庫存同步設定
    sync_stock BOOLEAN DEFAULT false, -- 是否同步庫存
    stock_buffer INT DEFAULT 0, -- 庫存緩衝（外賣平台顯示 = 實際庫存 - buffer）
    
    -- 狀態
    is_active BOOLEAN DEFAULT true,
    last_synced_at TIMESTAMP WITH TIME ZONE,
    
    -- 審計
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- 唯一約束：同一平台同一商品只能映射一次
    CONSTRAINT uq_delivery_product_mapping UNIQUE (tenant_id, platform, platform_item_id)
);

CREATE INDEX IF NOT EXISTS idx_delivery_product_mappings_tenant ON delivery_product_mappings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_delivery_product_mappings_platform ON delivery_product_mappings(platform, platform_item_id);
CREATE INDEX IF NOT EXISTS idx_delivery_product_mappings_product ON delivery_product_mappings(product_id);

COMMENT ON TABLE delivery_product_mappings IS '外賣平台商品映射表（可選：將平台商品關聯到 vWork 產品以追蹤庫存）';

-- 7. 清理
DROP TABLE IF EXISTS delivery_orders_backup;

-- 8. 添加 Comment 說明
COMMENT ON TABLE delivery_order_details IS '外賣訂單補充資訊（主體數據在 orders 表）';
COMMENT ON COLUMN orders.delivery_platform IS '外賣平台：foodpanda, keeta, deliveroo';
COMMENT ON COLUMN orders.platform_order_id IS '外賣平台訂單編號';
COMMENT ON COLUMN orders.source_type IS '訂單來源：erp, pos, webstore, delivery';
