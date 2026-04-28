-- Create shifts table
-- 工作時段表：定義不同的工作時段（例如：早班、中班、晚班）

CREATE TABLE IF NOT EXISTS shifts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,  -- 時段名稱，例如：早班、中班、晚班
    start_time TIME NOT NULL,    -- 上班時間，例如：09:00
    end_time TIME NOT NULL,       -- 下班時間，例如：18:00
    is_default BOOLEAN DEFAULT false,  -- 是否為預設時段
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shifts_tenant_id ON shifts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_shifts_is_default ON shifts(tenant_id, is_default);

COMMENT ON TABLE shifts IS '工作時段表：定義不同的工作時段';
COMMENT ON COLUMN shifts.name IS '時段名稱，例如：早班、中班、晚班';
COMMENT ON COLUMN shifts.start_time IS '上班時間，例如：09:00';
COMMENT ON COLUMN shifts.end_time IS '下班時間，例如：18:00';
COMMENT ON COLUMN shifts.is_default IS '是否為預設時段';

-- Add shift_id to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS shift_id UUID REFERENCES shifts(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_users_shift_id ON users(shift_id);

COMMENT ON COLUMN users.shift_id IS '關聯的工作時段 ID';




