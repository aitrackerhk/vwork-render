-- Create rental_orders table
CREATE TABLE IF NOT EXISTS rental_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    order_number VARCHAR(100) NOT NULL,
    customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
    rental_date DATE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    total_amount DECIMAL(15,2) DEFAULT 0,
    coupon_id UUID REFERENCES coupons(id) ON DELETE SET NULL,
    points_used INT DEFAULT 0,
    points_earned INT DEFAULT 0,
    points_discount DECIMAL(18,2) DEFAULT 0.00,
    coupon_discount DECIMAL(18,2) DEFAULT 0.00,
    referral_code VARCHAR(50),
    contact_name VARCHAR(255),
    contact_email VARCHAR(255),
    contact_phone VARCHAR(50),
    contact_address TEXT,
    salesperson_id UUID REFERENCES users(id) ON DELETE SET NULL,
    store_id UUID REFERENCES stores(id) ON DELETE SET NULL,
    commission_amount DECIMAL(15,2) DEFAULT 0,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    trashed_at TIMESTAMP WITH TIME ZONE,
    extra_fields JSONB DEFAULT '{}',
    UNIQUE(tenant_id, order_number)
);

CREATE INDEX IF NOT EXISTS idx_rental_orders_tenant_id ON rental_orders(tenant_id);
CREATE INDEX IF NOT EXISTS idx_rental_orders_customer_id ON rental_orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_rental_orders_status ON rental_orders(status);
CREATE INDEX IF NOT EXISTS idx_rental_orders_rental_date ON rental_orders(rental_date);

-- Create rental_order_items table
CREATE TABLE IF NOT EXISTS rental_order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rental_order_id UUID NOT NULL REFERENCES rental_orders(id) ON DELETE CASCADE,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID,
    resource_name VARCHAR(255),
    staff_id UUID REFERENCES users(id) ON DELETE SET NULL,
    quantity DECIMAL(10,2) NOT NULL,
    unit_price DECIMAL(15,2) NOT NULL,
    total_price DECIMAL(15,2) NOT NULL,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    trashed_at TIMESTAMP WITH TIME ZONE,
    extra_fields JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_rental_order_items_tenant_id ON rental_order_items(tenant_id);
CREATE INDEX IF NOT EXISTS idx_rental_order_items_rental_order_id ON rental_order_items(rental_order_id);

-- Create rental_order_labels table
CREATE TABLE IF NOT EXISTS rental_order_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#007bff',
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    trashed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_rental_order_labels_tenant_id ON rental_order_labels(tenant_id);

-- Create rental_order_label_relations join table
CREATE TABLE IF NOT EXISTS rental_order_label_relations (
    rental_order_id UUID NOT NULL REFERENCES rental_orders(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES rental_order_labels(id) ON DELETE CASCADE,
    PRIMARY KEY (rental_order_id, label_id)
);

-- Add rental_order_id column to appointments table
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS rental_order_id UUID REFERENCES rental_orders(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_appointments_rental_order_id ON appointments(rental_order_id);
