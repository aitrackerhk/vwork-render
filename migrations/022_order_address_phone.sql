-- Add address and phone fields to orders table
ALTER TABLE orders ADD COLUMN IF NOT EXISTS contact_phone VARCHAR(50);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS contact_address TEXT;

