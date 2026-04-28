ALTER TABLE IF EXISTS dining_queues
  ADD COLUMN IF NOT EXISTS reservation_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_dining_queues_reservation_at ON dining_queues (reservation_at);
