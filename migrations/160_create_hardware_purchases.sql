-- Create hardware_purchases table
CREATE TABLE IF NOT EXISTS hardware_purchases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    user_id uuid NULL,
    status varchar(50) NOT NULL DEFAULT 'created',
    checkout_session_id varchar(255) NULL,
    payment_intent_id varchar(255) NULL,
    stripe_customer_id varchar(255) NULL,
    currency varchar(20) NULL,
    amount_total numeric(12,2) NOT NULL DEFAULT 0,
    items jsonb NOT NULL DEFAULT '[]',
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_hardware_purchases_tenant_id ON hardware_purchases(tenant_id);
CREATE INDEX IF NOT EXISTS idx_hardware_purchases_created_at ON hardware_purchases(created_at DESC);
