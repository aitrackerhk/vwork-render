-- Migration 194: Add product column to api_tokens table
-- Product identifies which V-sys product this token belongs to (vai, vwork, vmarket, voffice)

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'api_tokens' AND column_name = 'product'
    ) THEN
        ALTER TABLE api_tokens ADD COLUMN product VARCHAR(20) NOT NULL DEFAULT 'vwork';
    END IF;
END $$;

-- Add index on product for filtering
CREATE INDEX IF NOT EXISTS idx_api_tokens_product ON api_tokens (product);
