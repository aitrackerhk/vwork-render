-- Sales Partner pricing fields

ALTER TABLE sales_partner_applications
    ADD COLUMN IF NOT EXISTS monthly_price DECIMAL(10,2) NULL,
    ADD COLUMN IF NOT EXISTS yearly_price DECIMAL(10,2) NULL,
    ADD COLUMN IF NOT EXISTS currency VARCHAR(10) NULL,
    ADD COLUMN IF NOT EXISTS stripe_price_id_monthly VARCHAR(255) NULL,
    ADD COLUMN IF NOT EXISTS stripe_price_id_yearly VARCHAR(255) NULL;

CREATE INDEX IF NOT EXISTS idx_sales_partner_applications_pricing ON sales_partner_applications(monthly_price, yearly_price);
