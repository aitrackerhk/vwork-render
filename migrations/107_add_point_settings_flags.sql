-- Add new point settings flags for order/service order earn points and referral bonus
ALTER TABLE point_settings
    ADD COLUMN IF NOT EXISTS enable_order_earn_points BOOLEAN DEFAULT TRUE;

ALTER TABLE point_settings
    ADD COLUMN IF NOT EXISTS enable_service_order_earn_points BOOLEAN DEFAULT TRUE;

ALTER TABLE point_settings
    ADD COLUMN IF NOT EXISTS enable_order_referral_bonus BOOLEAN DEFAULT TRUE;

ALTER TABLE point_settings
    ADD COLUMN IF NOT EXISTS enable_service_order_referral_bonus BOOLEAN DEFAULT TRUE;

-- Backfill: set to TRUE for existing records
UPDATE point_settings
SET
    enable_order_earn_points = COALESCE(enable_order_earn_points, TRUE),
    enable_service_order_earn_points = COALESCE(enable_service_order_earn_points, TRUE),
    enable_order_referral_bonus = COALESCE(enable_order_referral_bonus, TRUE),
    enable_service_order_referral_bonus = COALESCE(enable_service_order_referral_bonus, TRUE)
WHERE TRUE;

