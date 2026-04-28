-- 創建車輛表
CREATE TABLE IF NOT EXISTS vehicles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE NOT NULL,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    vehicle_type VARCHAR(100),
    license_plate VARCHAR(50),
    status VARCHAR(50) DEFAULT 'available',
    allow_overlap BOOLEAN DEFAULT false,
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 創建預約車輛關聯表
CREATE TABLE IF NOT EXISTS appointment_vehicles (
    appointment_id UUID REFERENCES appointments(id) ON DELETE CASCADE,
    vehicle_id UUID REFERENCES vehicles(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (appointment_id, vehicle_id)
);

-- 創建索引
CREATE INDEX IF NOT EXISTS idx_vehicles_tenant_id ON vehicles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_vehicles_code ON vehicles(code);
CREATE INDEX IF NOT EXISTS idx_appointment_vehicles_appointment_id ON appointment_vehicles(appointment_id);
CREATE INDEX IF NOT EXISTS idx_appointment_vehicles_vehicle_id ON appointment_vehicles(vehicle_id);

