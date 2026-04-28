-- 房間和設備分離，預約支持開始和結束時間

-- 創建房間表
CREATE TABLE IF NOT EXISTS rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE NOT NULL,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    description TEXT,
    capacity INTEGER,
    status VARCHAR(50) DEFAULT 'available',
    allow_overlap BOOLEAN DEFAULT false,
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 創建設備表
CREATE TABLE IF NOT EXISTS equipments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE NOT NULL,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    equipment_type VARCHAR(100),
    status VARCHAR(50) DEFAULT 'available',
    allow_overlap BOOLEAN DEFAULT false,
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 修改 appointments 表，添加開始和結束時間
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS start_time TIMESTAMP;
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS end_time TIMESTAMP;
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS reminder_time TIMESTAMP;

-- 創建預約房間關聯表
CREATE TABLE IF NOT EXISTS appointment_rooms (
    appointment_id UUID REFERENCES appointments(id) ON DELETE CASCADE,
    room_id UUID REFERENCES rooms(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (appointment_id, room_id)
);

-- 創建預約設備關聯表
CREATE TABLE IF NOT EXISTS appointment_equipments (
    appointment_id UUID REFERENCES appointments(id) ON DELETE CASCADE,
    equipment_id UUID REFERENCES equipments(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (appointment_id, equipment_id)
);

-- 創建索引
CREATE INDEX IF NOT EXISTS idx_rooms_tenant_id ON rooms(tenant_id);
CREATE INDEX IF NOT EXISTS idx_rooms_status ON rooms(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_equipments_tenant_id ON equipments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_equipments_status ON equipments(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_appointments_start_time ON appointments(tenant_id, start_time);
CREATE INDEX IF NOT EXISTS idx_appointments_end_time ON appointments(tenant_id, end_time);
CREATE INDEX IF NOT EXISTS idx_appointment_rooms_room_id ON appointment_rooms(room_id);
CREATE INDEX IF NOT EXISTS idx_appointment_equipments_equipment_id ON appointment_equipments(equipment_id);

