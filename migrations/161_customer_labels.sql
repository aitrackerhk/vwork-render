-- Create customer_labels table
CREATE TABLE IF NOT EXISTS customer_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#007bff',
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    trashed_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(tenant_id, name)
);

-- Create customer_label_relations table (many-to-many)
CREATE TABLE IF NOT EXISTS customer_label_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES customer_labels(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(customer_id, label_id)
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_customer_labels_tenant ON customer_labels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_customer_label_relations_customer ON customer_label_relations(customer_id);
CREATE INDEX IF NOT EXISTS idx_customer_label_relations_label ON customer_label_relations(label_id);
