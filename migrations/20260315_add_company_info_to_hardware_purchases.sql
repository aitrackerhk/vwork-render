-- Add company_info JSONB column to hardware_purchases table
-- Stores tailor-made website company info (company_link, file paths) submitted by user

ALTER TABLE hardware_purchases
ADD COLUMN IF NOT EXISTS company_info jsonb DEFAULT NULL;

COMMENT ON COLUMN hardware_purchases.company_info IS 'JSONB object with company_link (string) and files (array of file URLs) for tailor-made website requests';
