-- 185_stripe_connect.sql
-- Add Stripe Connect support: each tenant can have a connected Stripe account

-- Add stripe_connect_account_id to tenants
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS stripe_connect_account_id VARCHAR(255);
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS stripe_connect_onboarding_complete BOOLEAN DEFAULT FALSE;

-- Index for quick lookup
CREATE INDEX IF NOT EXISTS idx_tenants_stripe_connect_account_id ON tenants(stripe_connect_account_id) WHERE stripe_connect_account_id IS NOT NULL;
