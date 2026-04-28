-- Add substance_category to products
ALTER TABLE products ADD COLUMN IF NOT EXISTS substance_category VARCHAR(100);
