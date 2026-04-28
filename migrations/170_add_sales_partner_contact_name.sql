-- Add contact name to sales partner applications

ALTER TABLE sales_partner_applications
    ADD COLUMN IF NOT EXISTS contact_name VARCHAR(100) NOT NULL DEFAULT '';
