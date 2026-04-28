-- Add block_id to page_components for referenced/linked blocks
-- When block_id is set, the component renders using the block's latest data
ALTER TABLE page_components ADD COLUMN IF NOT EXISTS block_id UUID REFERENCES blocks(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_page_components_block_id ON page_components(block_id) WHERE block_id IS NOT NULL;
