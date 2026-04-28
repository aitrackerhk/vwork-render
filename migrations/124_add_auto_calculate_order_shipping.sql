-- Add auto_calculate_order_shipping to document_auto_settings
ALTER TABLE document_auto_settings
ADD COLUMN IF NOT EXISTS auto_calculate_order_shipping BOOLEAN NOT NULL DEFAULT FALSE;

-- Backfill nulls (safety)
UPDATE document_auto_settings
SET auto_calculate_order_shipping = FALSE
WHERE auto_calculate_order_shipping IS NULL;


