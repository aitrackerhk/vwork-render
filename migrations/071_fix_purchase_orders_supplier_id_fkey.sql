-- 修改 purchase_orders 表的 supplier_id 外鍵，從 customers 改為 suppliers
-- 首先刪除舊的外鍵約束
ALTER TABLE purchase_orders 
    DROP CONSTRAINT IF EXISTS purchase_orders_supplier_id_fkey;

-- 添加新的外鍵約束，指向 suppliers 表
ALTER TABLE purchase_orders 
    ADD CONSTRAINT purchase_orders_supplier_id_fkey 
    FOREIGN KEY (supplier_id) 
    REFERENCES suppliers(id) 
    ON DELETE SET NULL;

