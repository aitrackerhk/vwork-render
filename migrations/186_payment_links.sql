-- 186_payment_links.sql
-- Payment Links: allow merchants to generate payment links for existing orders
-- Customers can pay via Stripe or PayPal without needing an account

CREATE TABLE IF NOT EXISTS payment_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    order_id        UUID NOT NULL REFERENCES orders(id),
    token           VARCHAR(64) NOT NULL UNIQUE,
    status          VARCHAR(20) NOT NULL DEFAULT 'active',  -- active, paid, expired, cancelled
    expires_at      TIMESTAMPTZ,
    notes           TEXT DEFAULT '',
    created_by      UUID REFERENCES users(id),
    extra_fields    JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_links_tenant_id ON payment_links(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payment_links_order_id ON payment_links(order_id);
CREATE INDEX IF NOT EXISTS idx_payment_links_token ON payment_links(token);
CREATE INDEX IF NOT EXISTS idx_payment_links_status ON payment_links(status);
