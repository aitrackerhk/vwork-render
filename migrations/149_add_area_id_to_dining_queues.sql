ALTER TABLE IF EXISTS dining_queues ADD COLUMN IF NOT EXISTS area_id uuid;
CREATE INDEX IF NOT EXISTS idx_dining_queues_area_id ON dining_queues (area_id);
