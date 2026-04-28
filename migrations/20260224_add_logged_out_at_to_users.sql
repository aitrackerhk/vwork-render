-- SSO cross-domain logout: Add logged_out_at column to users table.
-- When a user logs out from any product, logged_out_at is set to NOW().
-- Auth middleware rejects JWT tokens whose iat (IssuedAt) < logged_out_at,
-- effectively invalidating all sessions across all domains.
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS logged_out_at TIMESTAMP WITH TIME ZONE;
