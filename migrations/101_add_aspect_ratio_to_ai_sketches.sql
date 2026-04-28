-- Add aspect_ratio column to ai_sketches table
-- Stores the canvas aspect ratio (e.g. '3:4', '7:9', '16:9') or 'free'

ALTER TABLE ai_sketches
ADD COLUMN IF NOT EXISTS aspect_ratio VARCHAR(10) DEFAULT 'free';

COMMENT ON COLUMN ai_sketches.aspect_ratio IS 'Canvas aspect ratio string, e.g. 3:4, 7:9, 16:9, or free';
