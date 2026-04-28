-- 餐飲：桌區 / 餐桌 / 排隊

CREATE TABLE IF NOT EXISTS dining_areas (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    store_id uuid NULL,
    code varchar(50) NOT NULL,
    name varchar(100) NOT NULL,
    min_seats int DEFAULT 1,
    max_seats int DEFAULT 1,
    sort_order int DEFAULT 0,
    is_active boolean DEFAULT true,
    notes text,
    created_at timestamptz DEFAULT now(),
    updated_at timestamptz DEFAULT now(),
    created_by uuid NULL,
    updated_by uuid NULL
);

CREATE INDEX IF NOT EXISTS idx_dining_areas_tenant_id ON dining_areas(tenant_id);
CREATE INDEX IF NOT EXISTS idx_dining_areas_store_id ON dining_areas(store_id);
CREATE INDEX IF NOT EXISTS idx_dining_areas_code ON dining_areas(code);

CREATE TABLE IF NOT EXISTS dining_tables (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    store_id uuid NULL,
    area_id uuid NULL,
    code varchar(50) NOT NULL,
    name varchar(100),
    seats int DEFAULT 1,
    status varchar(20) DEFAULT 'available',
    is_active boolean DEFAULT true,
    notes text,
    created_at timestamptz DEFAULT now(),
    updated_at timestamptz DEFAULT now(),
    created_by uuid NULL,
    updated_by uuid NULL
);

CREATE INDEX IF NOT EXISTS idx_dining_tables_tenant_id ON dining_tables(tenant_id);
CREATE INDEX IF NOT EXISTS idx_dining_tables_store_id ON dining_tables(store_id);
CREATE INDEX IF NOT EXISTS idx_dining_tables_area_id ON dining_tables(area_id);
CREATE INDEX IF NOT EXISTS idx_dining_tables_code ON dining_tables(code);

CREATE TABLE IF NOT EXISTS dining_queues (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    store_id uuid NULL,
    name varchar(100) NOT NULL,
    phone varchar(50),
    party_size int DEFAULT 1,
    status varchar(20) DEFAULT 'waiting',
    table_id uuid NULL,
    table_code varchar(50),
    notes text,
    created_at timestamptz DEFAULT now(),
    updated_at timestamptz DEFAULT now(),
    seated_at timestamptz NULL,
    cancelled_at timestamptz NULL,
    created_by uuid NULL,
    updated_by uuid NULL
);

CREATE INDEX IF NOT EXISTS idx_dining_queues_tenant_id ON dining_queues(tenant_id);
CREATE INDEX IF NOT EXISTS idx_dining_queues_store_id ON dining_queues(store_id);
CREATE INDEX IF NOT EXISTS idx_dining_queues_status ON dining_queues(status);
CREATE INDEX IF NOT EXISTS idx_dining_queues_table_id ON dining_queues(table_id);
