-- Migration: Add vSuite and vSuite Pro subscription plans
-- Replaces old "monthly"/"yearly" plans with new 4-tier naming:
--   vsuite_monthly ($380/mo), vsuite_yearly ($250/mo billed yearly = $3000/yr)
--   vsuite_pro_monthly ($680/mo), vsuite_pro_yearly ($500/mo billed yearly = $6000/yr)

-- 1. Rename old plans to new names (preserve UUIDs and existing subscriptions)
UPDATE subscription_plans SET
    name = 'vsuite_monthly',
    display_name = 'vSuite 月付方案',
    price = 380.00,
    yearly_price = NULL,
    interval = 'month',
    is_active = true,
    updated_at = CURRENT_TIMESTAMP
WHERE name = 'monthly';

UPDATE subscription_plans SET
    name = 'vsuite_yearly',
    display_name = 'vSuite 年付方案',
    price = 250.00,
    yearly_price = 3000.00,
    interval = 'year',
    is_active = true,
    updated_at = CURRENT_TIMESTAMP
WHERE name = 'yearly';

-- 2. Insert vSuite Pro plans (only if they don't already exist)
INSERT INTO subscription_plans (name, display_name, price, yearly_price, interval, is_active, stripe_price_id)
VALUES
    ('vsuite_pro_monthly', 'vSuite Pro 月付方案', 680.00, NULL, 'month', true, NULL),
    ('vsuite_pro_yearly', 'vSuite Pro 年付方案', 500.00, 6000.00, 'year', true, NULL)
ON CONFLICT (name) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    price = EXCLUDED.price,
    yearly_price = EXCLUDED.yearly_price,
    interval = EXCLUDED.interval,
    is_active = true,
    updated_at = CURRENT_TIMESTAMP;

-- 3. Update tenant plan names from legacy to new names
UPDATE tenants SET plan = 'vsuite_monthly' WHERE plan = 'monthly';
UPDATE tenants SET plan = 'vsuite_yearly' WHERE plan = 'yearly';
