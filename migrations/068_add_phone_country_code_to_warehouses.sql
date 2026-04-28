-- Add phone_country_code to warehouses table
ALTER TABLE warehouses
    ADD COLUMN IF NOT EXISTS phone_country_code VARCHAR(10) NULL;

