-- Add auto-generate payment (invoice) settings for orders and service orders
ALTER TABLE document_auto_settings
    ADD COLUMN IF NOT EXISTS auto_generate_order_payment BOOLEAN DEFAULT TRUE;

ALTER TABLE document_auto_settings
    ADD COLUMN IF NOT EXISTS auto_generate_service_order_payment BOOLEAN DEFAULT TRUE;

-- Backfill: set to TRUE for existing records
UPDATE document_auto_settings
SET
    auto_generate_order_payment = COALESCE(auto_generate_order_payment, TRUE),
    auto_generate_service_order_payment = COALESCE(auto_generate_service_order_payment, TRUE)
WHERE TRUE;

