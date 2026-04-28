-- Add logo and address fields to enterprises table
ALTER TABLE enterprises ADD COLUMN IF NOT EXISTS logo_url VARCHAR(500);
ALTER TABLE enterprises ADD COLUMN IF NOT EXISTS address TEXT;

