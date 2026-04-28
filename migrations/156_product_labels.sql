-- Create product_labels table
CREATE TABLE IF NOT EXISTS product_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#007bff', -- Hex color code
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    trashed_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(tenant_id, name)
);

-- Create product_label_relations table (many-to-many)
CREATE TABLE IF NOT EXISTS product_label_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES product_labels(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(product_id, label_id)
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_product_labels_tenant ON product_labels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_product_label_relations_product ON product_label_relations(product_id);
CREATE INDEX IF NOT EXISTS idx_product_label_relations_label ON product_label_relations(label_id);
