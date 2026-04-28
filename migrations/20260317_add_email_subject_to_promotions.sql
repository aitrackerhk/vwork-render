-- Add email_subject column to promotions table for email promotion type
ALTER TABLE promotions ADD COLUMN IF NOT EXISTS email_subject VARCHAR(255) DEFAULT '';
