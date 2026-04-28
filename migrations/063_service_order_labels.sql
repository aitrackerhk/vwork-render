-- Create service_order_labels table
CREATE TABLE IF NOT EXISTS service_order_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#007bff', -- Hex color code
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, name)
);

-- Create service_order_label_relations table (many-to-many)
CREATE TABLE IF NOT EXISTS service_order_label_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_order_id UUID NOT NULL REFERENCES service_orders(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES service_order_labels(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(service_order_id, label_id)
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_service_order_labels_tenant ON service_order_labels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_service_order_label_relations_service_order ON service_order_label_relations(service_order_id);
CREATE INDEX IF NOT EXISTS idx_service_order_label_relations_label ON service_order_label_relations(label_id);

-- Update service_orders model to use service_order_labels instead of order_labels
-- This is done by updating the many-to-many relationship in the model

