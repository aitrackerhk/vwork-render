-- Split commission auto-generation settings into:
-- - auto_generate_order_commission
-- - auto_generate_service_order_commission
-- Keep legacy column auto_generate_commission for backward compatibility.

ALTER TABLE document_auto_settings
    ADD COLUMN IF NOT EXISTS auto_generate_order_commission BOOLEAN DEFAULT TRUE;

ALTER TABLE document_auto_settings
    ADD COLUMN IF NOT EXISTS auto_generate_service_order_commission BOOLEAN DEFAULT TRUE;

-- Backfill from legacy setting if present
UPDATE document_auto_settings
SET
    auto_generate_order_commission = COALESCE(auto_generate_order_commission, auto_generate_commission),
    auto_generate_service_order_commission = COALESCE(auto_generate_service_order_commission, auto_generate_commission)
WHERE TRUE;


