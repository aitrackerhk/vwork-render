-- Migration: 184_add_delivery_product_mapping_fields.sql
-- Description: 為 delivery_product_mappings 表添加庫存同步相關欄位
-- Created: 2026-02-05

-- 添加 sync_inventory 欄位（是否扣減內部庫存）
ALTER TABLE delivery_product_mappings ADD COLUMN IF NOT EXISTS sync_inventory BOOLEAN DEFAULT false;

-- 添加 quantity_ratio 欄位（數量比例，例如平台1份=內部2份）
ALTER TABLE delivery_product_mappings ADD COLUMN IF NOT EXISTS quantity_ratio DECIMAL(10,4) DEFAULT 1;

-- 添加 Comment 說明
COMMENT ON COLUMN delivery_product_mappings.sync_inventory IS '是否在外賣訂單完成時扣減 vWork 內部庫存';
COMMENT ON COLUMN delivery_product_mappings.quantity_ratio IS '數量比例（例如：平台1份=內部2份，則設為2）';
