-- Create order_labels table
CREATE TABLE IF NOT EXISTS order_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#007bff', -- Hex color code
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, name)
);

-- Create order_label_relations table (many-to-many)
CREATE TABLE IF NOT EXISTS order_label_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES order_labels(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(order_id, label_id)
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_order_labels_tenant ON order_labels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_order_label_relations_order ON order_label_relations(order_id);
CREATE INDEX IF NOT EXISTS idx_order_label_relations_label ON order_label_relations(label_id);







