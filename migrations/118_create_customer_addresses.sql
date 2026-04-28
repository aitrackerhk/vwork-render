-- 創建客戶地址管理表
CREATE TABLE IF NOT EXISTS customer_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    country_code VARCHAR(10) NOT NULL,
    country_name VARCHAR(255) NOT NULL,
    region_code VARCHAR(50),
    region_name VARCHAR(255),
    postal_code VARCHAR(50),
    address_line1 TEXT NOT NULL,
    address_line2 TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_customer_addresses_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    CONSTRAINT fk_customer_addresses_customer FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_customer_addresses_tenant_id ON customer_addresses(tenant_id);
CREATE INDEX IF NOT EXISTS idx_customer_addresses_customer_id ON customer_addresses(customer_id);
CREATE INDEX IF NOT EXISTS idx_customer_addresses_is_default ON customer_addresses(customer_id, is_default);

COMMENT ON TABLE customer_addresses IS '客戶地址管理表';
COMMENT ON COLUMN customer_addresses.country_code IS '國家代碼（ISO 3166-1 alpha-2）';
COMMENT ON COLUMN customer_addresses.country_name IS '國家名稱';
COMMENT ON COLUMN customer_addresses.region_code IS '地區代碼';
COMMENT ON COLUMN customer_addresses.region_name IS '地區名稱';
COMMENT ON COLUMN customer_addresses.postal_code IS '郵政編碼';
COMMENT ON COLUMN customer_addresses.address_line1 IS '地址第一行';
COMMENT ON COLUMN customer_addresses.address_line2 IS '地址第二行（可選）';
COMMENT ON COLUMN customer_addresses.is_default IS '是否為默認地址';
