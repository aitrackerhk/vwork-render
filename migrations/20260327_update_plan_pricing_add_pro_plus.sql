-- Migration: Update vSuite Pro pricing and add vSuite Pro+ plans
-- vSuite Pro: $680/mo → $880/mo, $6,000/yr → $9,600/yr ($800/mo billed yearly)
-- vSuite Pro+: $2,500/mo, $26,400/yr ($2,200/mo billed yearly) — new public plan

-- 1. Update vSuite Pro pricing
UPDATE subscription_plans SET
    price = 880.00,
    updated_at = CURRENT_TIMESTAMP
WHERE name = 'vsuite_pro_monthly';

UPDATE subscription_plans SET
    price = 800.00,
    yearly_price = 9600.00,
    updated_at = CURRENT_TIMESTAMP
WHERE name = 'vsuite_pro_yearly';

-- 2. Insert vSuite Pro+ plans (only if they don't already exist)
INSERT INTO subscription_plans (name, display_name, price, yearly_price, interval, is_active, stripe_price_id)
VALUES
    ('vsuite_pro_plus_monthly', 'vSuite Pro+ 月付方案', 2500.00, NULL, 'month', true, NULL),
    ('vsuite_pro_plus_yearly', 'vSuite Pro+ 年付方案', 2200.00, 26400.00, 'year', true, NULL)
ON CONFLICT (name) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    price = EXCLUDED.price,
    yearly_price = EXCLUDED.yearly_price,
    interval = EXCLUDED.interval,
    is_active = true,
    updated_at = CURRENT_TIMESTAMP;
