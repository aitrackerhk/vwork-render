-- 187_sso_tickets.sql
-- SSO 跨域認證票據表
-- 用途：當用戶從一個產品域名跳轉到另一個域名時，
--       透過一次性票據（ticket）傳遞認證狀態，實現跨域單點登入。

CREATE TABLE IF NOT EXISTS sso_tickets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   UUID REFERENCES tenants(id) ON DELETE SET NULL,
    ticket      VARCHAR(128) NOT NULL UNIQUE,
    used        BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sso_tickets_ticket ON sso_tickets(ticket);
CREATE INDEX IF NOT EXISTS idx_sso_tickets_expires_at ON sso_tickets(expires_at);
