-- Add auto_generate_purchase_order_expense to document_auto_settings (default ON)
ALTER TABLE document_auto_settings
ADD COLUMN IF NOT EXISTS auto_generate_purchase_order_expense BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE document_auto_settings
SET auto_generate_purchase_order_expense = TRUE
WHERE auto_generate_purchase_order_expense IS NULL;


