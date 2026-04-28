-- Add IAP (In-App Purchase) support for Google Play and App Store
-- Extends subscription system to support 3 payment providers: stripe, google, apple

-- Add payment_provider column to subscriptions
ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS payment_provider VARCHAR(20) DEFAULT 'stripe',
    ADD COLUMN IF NOT EXISTS google_purchase_token TEXT,
    ADD COLUMN IF NOT EXISTS google_order_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_original_transaction_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_transaction_id VARCHAR(255);

-- Add IAP product IDs to subscription_plans
ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS google_product_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS apple_product_id VARCHAR(255);

-- Create index for IAP lookups
CREATE INDEX IF NOT EXISTS idx_subscriptions_google_order_id
    ON subscriptions(google_order_id) WHERE google_order_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_subscriptions_apple_original_transaction_id
    ON subscriptions(apple_original_transaction_id) WHERE apple_original_transaction_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_subscriptions_payment_provider
    ON subscriptions(payment_provider);

-- Update subscription_plans with IAP product IDs
-- These must match the product IDs configured in Google Play Console and App Store Connect
UPDATE subscription_plans SET
    google_product_id = 'vsuite_monthly',
    apple_product_id = 'com.vsys.vai.vsuite.monthly'
WHERE name = 'vsuite_monthly';

UPDATE subscription_plans SET
    google_product_id = 'vsuite_yearly',
    apple_product_id = 'com.vsys.vai.vsuite.yearly'
WHERE name = 'vsuite_yearly';

UPDATE subscription_plans SET
    google_product_id = 'vsuite_pro_monthly',
    apple_product_id = 'com.vsys.vai.vsuite.pro.monthly'
WHERE name = 'vsuite_pro_monthly';

UPDATE subscription_plans SET
    google_product_id = 'vsuite_pro_yearly',
    apple_product_id = 'com.vsys.vai.vsuite.pro.yearly'
WHERE name = 'vsuite_pro_yearly';

-- Create table for IAP purchase records (audit trail)
CREATE TABLE IF NOT EXISTS iap_purchases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    platform VARCHAR(20) NOT NULL, -- 'google' or 'apple'
    product_id VARCHAR(255) NOT NULL,
    purchase_type VARCHAR(20) NOT NULL, -- 'subscription' or 'consumable'
    -- Google Play fields
    google_purchase_token TEXT,
    google_order_id VARCHAR(255),
    -- Apple fields
    apple_transaction_id VARCHAR(255),
    apple_original_transaction_id VARCHAR(255),
    apple_environment VARCHAR(20), -- 'Production' or 'Sandbox'
    -- Common fields
    status VARCHAR(50) NOT NULL, -- 'purchased', 'pending', 'refunded', 'expired', 'cancelled'
    amount DECIMAL(10,2),
    currency VARCHAR(10),
    verified_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    raw_receipt JSONB, -- full receipt data for debugging
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_iap_purchases_tenant_id ON iap_purchases(tenant_id);
CREATE INDEX IF NOT EXISTS idx_iap_purchases_platform ON iap_purchases(platform);
CREATE INDEX IF NOT EXISTS idx_iap_purchases_google_order_id
    ON iap_purchases(google_order_id) WHERE google_order_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_iap_purchases_apple_transaction_id
    ON iap_purchases(apple_transaction_id) WHERE apple_transaction_id IS NOT NULL;
