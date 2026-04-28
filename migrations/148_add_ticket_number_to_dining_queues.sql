-- Add ticket number and sequence for dining queues
ALTER TABLE IF EXISTS dining_queues ADD COLUMN IF NOT EXISTS ticket_number VARCHAR(50);
ALTER TABLE IF EXISTS dining_queues ADD COLUMN IF NOT EXISTS ticket_seq INTEGER;

CREATE INDEX IF NOT EXISTS idx_dining_queues_ticket_number ON dining_queues (ticket_number);
CREATE INDEX IF NOT EXISTS idx_dining_queues_ticket_seq ON dining_queues (ticket_seq);
CREATE INDEX IF NOT EXISTS idx_dining_queues_tenant_ticket ON dining_queues (tenant_id, ticket_number);
