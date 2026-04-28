-- 每租戶 / 每頁 / 每日 瀏覽量累計
-- 用於 vBuilder「瀏覽報告」(每頁瀏覽量)

CREATE TABLE IF NOT EXISTS page_view_daily (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    view_date date NOT NULL,
    view_count integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- 同一天同頁只保留一筆，使用 UPSERT 累加
CREATE UNIQUE INDEX IF NOT EXISTS uq_page_view_daily_tenant_page_date
    ON page_view_daily(tenant_id, page_id, view_date);

CREATE INDEX IF NOT EXISTS idx_page_view_daily_tenant_date
    ON page_view_daily(tenant_id, view_date);


