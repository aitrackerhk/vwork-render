-- Platform-aware logout: split logged_out_at into per-platform columns.
-- Web logout only invalidates web tokens; desktop logout only invalidates desktop tokens.
-- The original logged_out_at column is kept for backward compatibility with legacy tokens.
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS web_logged_out_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS desktop_logged_out_at TIMESTAMP WITH TIME ZONE;
