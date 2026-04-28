-- Add attachments column to email_jobs table
-- The attachments column stores JSONB data for email attachments

ALTER TABLE email_jobs 
ADD COLUMN IF NOT EXISTS attachments JSONB DEFAULT '[]'::jsonb;

-- Add comment for documentation
COMMENT ON COLUMN email_jobs.attachments IS 'JSONB array of email attachments with name, content_type, and data fields';
