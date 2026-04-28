-- 為 enterprises 補上 tenant_id 與 domain 欄位，並關聯 tenants
ALTER TABLE enterprises
    ADD COLUMN IF NOT EXISTS tenant_id UUID UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS domain VARCHAR(255);

-- 將既有企業的 tenant_id 與 domain 由租戶補齊（假設名稱對應）
UPDATE enterprises e
SET tenant_id = t.id,
    domain = COALESCE(e.domain, t.subdomain)
FROM tenants t
WHERE (e.tenant_id IS NULL OR e.domain IS NULL)
  AND LOWER(e.name) = LOWER(t.name);

-- 對 tenant_id 建唯一索引確保一租戶一企業
CREATE UNIQUE INDEX IF NOT EXISTS idx_enterprises_tenant ON enterprises(tenant_id);

