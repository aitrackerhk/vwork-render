-- Create purchase_order_labels table
CREATE TABLE IF NOT EXISTS purchase_order_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#007bff', -- Hex color code
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, name)
);

-- Create purchase_order_label_relations table (many-to-many)
CREATE TABLE IF NOT EXISTS purchase_order_label_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_order_id UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES purchase_order_labels(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(purchase_order_id, label_id)
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_purchase_order_labels_tenant ON purchase_order_labels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_purchase_order_label_relations_purchase_order ON purchase_order_label_relations(purchase_order_id);
CREATE INDEX IF NOT EXISTS idx_purchase_order_label_relations_label ON purchase_order_label_relations(label_id);

