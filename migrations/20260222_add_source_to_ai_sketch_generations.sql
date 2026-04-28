-- Add source column to ai_sketch_generations
-- Values: 'sketch' (from vai-sketch tool) or 'chat' (from vai-chat image generation)

ALTER TABLE ai_sketch_generations
    ADD COLUMN IF NOT EXISTS source VARCHAR(20) DEFAULT 'sketch';

-- Index for source-based filtering
CREATE INDEX IF NOT EXISTS idx_ai_sketch_generations_source ON ai_sketch_generations(source);
