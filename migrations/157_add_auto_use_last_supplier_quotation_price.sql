-- Add auto_use_last_supplier_quotation_price to document_auto_settings (default ON)
-- When enabled, purchase order item unit price defaults to the latest quotation unit price
-- for the same supplier (customer_id) and product.

ALTER TABLE document_auto_settings
ADD COLUMN IF NOT EXISTS auto_use_last_supplier_quotation_price BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE document_auto_settings
SET auto_use_last_supplier_quotation_price = TRUE
WHERE auto_use_last_supplier_quotation_price IS NULL;
