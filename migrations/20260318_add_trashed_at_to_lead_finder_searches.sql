-- Add soft delete (trash bin) support to lead_finder_searches
ALTER TABLE lead_finder_searches ADD COLUMN IF NOT EXISTS trashed_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_lead_finder_searches_trashed ON lead_finder_searches(tenant_id, trashed_at) WHERE trashed_at IS NOT NULL;
