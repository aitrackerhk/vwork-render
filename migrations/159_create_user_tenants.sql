-- Create user_tenants table to support multi-tenant users
CREATE TABLE IF NOT EXISTS user_tenants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role varchar(50),
    is_default boolean DEFAULT false,
    last_used_at timestamp,
    created_at timestamp DEFAULT now(),
    updated_at timestamp DEFAULT now(),
    UNIQUE (user_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_user_tenants_user_id ON user_tenants(user_id);
CREATE INDEX IF NOT EXISTS idx_user_tenants_tenant_id ON user_tenants(tenant_id);

-- Backfill existing single-tenant users
INSERT INTO user_tenants (user_id, tenant_id, role, is_default, last_used_at, created_at, updated_at)
SELECT u.id, u.tenant_id, u.user_role, true, now(), now(), now()
FROM users u
WHERE u.tenant_id IS NOT NULL
  AND u.tenant_id <> '00000000-0000-0000-0000-000000000000'
  AND NOT EXISTS (
    SELECT 1 FROM user_tenants ut WHERE ut.user_id = u.id AND ut.tenant_id = u.tenant_id
  );
