-- Add salary_mode to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS salary_mode VARCHAR(20) DEFAULT 'monthly';

-- Backfill: set existing users to 'monthly' if salary > 0
UPDATE users SET salary_mode = 'monthly' WHERE salary_mode IS NULL AND salary > 0;

