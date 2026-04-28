-- project_resource_reservations：項目資源預留
CREATE TABLE IF NOT EXISTS project_resource_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    resource_type VARCHAR(50) NOT NULL, -- room, equipment, vehicle
    resource_id UUID NOT NULL,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- active, cancelled
    notes TEXT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    extra_fields JSONB DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_prr_tenant ON project_resource_reservations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_prr_project ON project_resource_reservations(project_id);
CREATE INDEX IF NOT EXISTS idx_prr_resource ON project_resource_reservations(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_prr_time ON project_resource_reservations(start_time, end_time);


