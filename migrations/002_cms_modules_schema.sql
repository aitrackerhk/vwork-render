-- vWork CMS 完整模組資料庫結構
-- 重建資料庫時使用此文件

-- ============================================
-- 設定模組
-- ============================================

-- 企業表
CREATE TABLE IF NOT EXISTS enterprises (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50) UNIQUE,
    domain VARCHAR(255),
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 公司表
CREATE TABLE IF NOT EXISTS companies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    enterprise_id UUID REFERENCES enterprises(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 部門表（含權限設定）
CREATE TABLE IF NOT EXISTS departments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID REFERENCES companies(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    parent_id UUID REFERENCES departments(id) ON DELETE SET NULL,
    permissions JSONB DEFAULT '[]', -- 權限列表
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 級別表（在部門下，含權限設定）
CREATE TABLE IF NOT EXISTS levels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    department_id UUID REFERENCES departments(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    level_order INTEGER DEFAULT 0,
    permissions JSONB DEFAULT '[]', -- 權限列表
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 地區表
CREATE TABLE IF NOT EXISTS regions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50) UNIQUE,
    parent_id UUID REFERENCES regions(id) ON DELETE SET NULL,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 貨幣表
CREATE TABLE IF NOT EXISTS currencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(10) UNIQUE NOT NULL, -- ISO 4217 貨幣代碼
    name VARCHAR(100) NOT NULL,
    symbol VARCHAR(10),
    exchange_rate DECIMAL(18, 6) DEFAULT 1.0,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 更新 users 表，添加級別關聯
ALTER TABLE users ADD COLUMN IF NOT EXISTS level_id UUID REFERENCES levels(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS department_id UUID REFERENCES departments(id) ON DELETE SET NULL;

-- ============================================
-- 個人模組
-- ============================================

-- 日曆表
CREATE TABLE IF NOT EXISTS calendars (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    event_type VARCHAR(50), -- meeting, deadline, reminder, holiday, etc.
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP,
    all_day BOOLEAN DEFAULT false,
    recurrence JSONB, -- 重複規則
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 提醒表
CREATE TABLE IF NOT EXISTS reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    remind_time TIMESTAMP NOT NULL,
    is_completed BOOLEAN DEFAULT false,
    completed_at TIMESTAMP,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 訊息表
CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    from_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    to_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    subject VARCHAR(255),
    content TEXT NOT NULL,
    is_read BOOLEAN DEFAULT false,
    read_at TIMESTAMP,
    message_type VARCHAR(50) DEFAULT 'normal', -- normal, system, notification
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 備忘表
CREATE TABLE IF NOT EXISTS notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    content TEXT,
    category VARCHAR(100),
    tags JSONB DEFAULT '[]',
    is_pinned BOOLEAN DEFAULT false,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 個人資料表
CREATE TABLE IF NOT EXISTS personal_data (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    data_type VARCHAR(100) NOT NULL, -- contact, document, credential, etc.
    key_name VARCHAR(255) NOT NULL,
    value TEXT,
    is_encrypted BOOLEAN DEFAULT false,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- 客戶模組擴展
-- ============================================

-- 會員等級表
CREATE TABLE IF NOT EXISTS member_levels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    level_order INTEGER DEFAULT 0,
    min_points INTEGER DEFAULT 0,
    discount_rate DECIMAL(5, 2) DEFAULT 0.00,
    benefits JSONB DEFAULT '{}',
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 積分表
CREATE TABLE IF NOT EXISTS points (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,
    points INTEGER NOT NULL,
    points_type VARCHAR(50) NOT NULL, -- earned, redeemed, expired, adjusted
    source_type VARCHAR(100), -- order, promotion, manual, etc.
    source_id UUID, -- 來源記錄ID
    description TEXT,
    expires_at TIMESTAMP,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 加盟表
CREATE TABLE IF NOT EXISTS franchises (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    contact_person VARCHAR(255),
    phone VARCHAR(50),
    email VARCHAR(255),
    address TEXT,
    region_id UUID REFERENCES regions(id) ON DELETE SET NULL,
    agreement_start DATE,
    agreement_end DATE,
    commission_rate DECIMAL(5, 2) DEFAULT 0.00,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 更新 customers 表，添加會員等級和積分
ALTER TABLE customers ADD COLUMN IF NOT EXISTS member_level_id UUID REFERENCES member_levels(id) ON DELETE SET NULL;
ALTER TABLE customers ADD COLUMN IF NOT EXISTS total_points INTEGER DEFAULT 0;
ALTER TABLE customers ADD COLUMN IF NOT EXISTS franchise_id UUID REFERENCES franchises(id) ON DELETE SET NULL;

-- ============================================
-- 產品模組擴展
-- ============================================

-- 產品類型表
CREATE TABLE IF NOT EXISTS product_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    parent_id UUID REFERENCES product_types(id) ON DELETE SET NULL,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 產品屬性表
CREATE TABLE IF NOT EXISTS product_attributes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    attribute_type VARCHAR(50) NOT NULL, -- text, number, select, boolean, etc.
    options JSONB, -- 選項列表（用於 select 類型）
    is_required BOOLEAN DEFAULT false,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 產品屬性值表（產品與屬性的關聯）
CREATE TABLE IF NOT EXISTS product_attribute_values (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID REFERENCES products(id) ON DELETE CASCADE,
    attribute_id UUID REFERENCES product_attributes(id) ON DELETE CASCADE,
    value TEXT,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(product_id, attribute_id)
);

-- 品牌表
CREATE TABLE IF NOT EXISTS brands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    logo_url VARCHAR(500),
    description TEXT,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 更新 products 表
ALTER TABLE products ADD COLUMN IF NOT EXISTS product_type_id UUID REFERENCES product_types(id) ON DELETE SET NULL;
ALTER TABLE products ADD COLUMN IF NOT EXISTS brand_id UUID REFERENCES brands(id) ON DELETE SET NULL;

-- ============================================
-- 服務模組
-- ============================================

-- 服務種類表
CREATE TABLE IF NOT EXISTS service_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    description TEXT,
    duration_minutes INTEGER,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 服務表
CREATE TABLE IF NOT EXISTS services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    service_type_id UUID REFERENCES service_types(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    description TEXT,
    price DECIMAL(18, 2) DEFAULT 0.00,
    duration_minutes INTEGER,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 預約表
CREATE TABLE IF NOT EXISTS appointments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,
    service_id UUID REFERENCES services(id) ON DELETE SET NULL,
    staff_id UUID REFERENCES users(id) ON DELETE SET NULL,
    appointment_date DATE NOT NULL,
    appointment_time TIME NOT NULL,
    duration_minutes INTEGER,
    notes TEXT,
    status VARCHAR(50) DEFAULT 'pending', -- pending, confirmed, completed, cancelled
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 服務單表
CREATE TABLE IF NOT EXISTS service_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,
    appointment_id UUID REFERENCES appointments(id) ON DELETE SET NULL,
    order_number VARCHAR(100) UNIQUE,
    service_date DATE NOT NULL,
    total_amount DECIMAL(18, 2) DEFAULT 0.00,
    discount_amount DECIMAL(18, 2) DEFAULT 0.00,
    final_amount DECIMAL(18, 2) DEFAULT 0.00,
    status VARCHAR(50) DEFAULT 'pending', -- pending, in_progress, completed, cancelled
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 服務單明細表
CREATE TABLE IF NOT EXISTS service_order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_order_id UUID REFERENCES service_orders(id) ON DELETE CASCADE,
    service_id UUID REFERENCES services(id) ON DELETE SET NULL,
    staff_id UUID REFERENCES users(id) ON DELETE SET NULL,
    quantity INTEGER DEFAULT 1,
    unit_price DECIMAL(18, 2) DEFAULT 0.00,
    discount_amount DECIMAL(18, 2) DEFAULT 0.00,
    total_amount DECIMAL(18, 2) DEFAULT 0.00,
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 服務員表（擴展用戶表）
CREATE TABLE IF NOT EXISTS service_staff (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    employee_number VARCHAR(50),
    specialization TEXT,
    hourly_rate DECIMAL(18, 2),
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, user_id)
);

-- 房間設備表
CREATE TABLE IF NOT EXISTS room_equipment (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    room_name VARCHAR(255) NOT NULL,
    equipment_name VARCHAR(255) NOT NULL,
    equipment_type VARCHAR(100),
    status VARCHAR(50) DEFAULT 'available', -- available, in_use, maintenance, unavailable
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- 訂單模組擴展
-- ============================================

-- 訂單報表表（用於存儲報表數據）
CREATE TABLE IF NOT EXISTS order_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    report_type VARCHAR(100) NOT NULL, -- daily, weekly, monthly, yearly, custom
    report_date DATE NOT NULL,
    total_orders INTEGER DEFAULT 0,
    total_amount DECIMAL(18, 2) DEFAULT 0.00,
    total_items INTEGER DEFAULT 0,
    report_data JSONB DEFAULT '{}',
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- 採購模組
-- ============================================

-- 採購單表
CREATE TABLE IF NOT EXISTS purchase_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    supplier_id UUID REFERENCES customers(id) ON DELETE SET NULL, -- 供應商也是客戶
    order_number VARCHAR(100) UNIQUE,
    order_date DATE NOT NULL,
    expected_delivery_date DATE,
    total_amount DECIMAL(18, 2) DEFAULT 0.00,
    discount_amount DECIMAL(18, 2) DEFAULT 0.00,
    tax_amount DECIMAL(18, 2) DEFAULT 0.00,
    final_amount DECIMAL(18, 2) DEFAULT 0.00,
    status VARCHAR(50) DEFAULT 'draft', -- draft, pending, approved, received, cancelled
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 採購單明細表
CREATE TABLE IF NOT EXISTS purchase_order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_order_id UUID REFERENCES purchase_orders(id) ON DELETE CASCADE,
    product_id UUID REFERENCES products(id) ON DELETE SET NULL,
    quantity INTEGER NOT NULL,
    unit_price DECIMAL(18, 2) DEFAULT 0.00,
    discount_amount DECIMAL(18, 2) DEFAULT 0.00,
    tax_rate DECIMAL(5, 2) DEFAULT 0.00,
    total_amount DECIMAL(18, 2) DEFAULT 0.00,
    received_quantity INTEGER DEFAULT 0,
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- 會計模組
-- ============================================

-- 收入表
CREATE TABLE IF NOT EXISTS incomes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    income_type VARCHAR(50) NOT NULL, -- order, invoice, service, other
    reference_id UUID, -- 關聯的訂單ID、發票ID等
    reference_type VARCHAR(50), -- order, invoice, service_order
    category VARCHAR(100), -- 收入類別
    description TEXT,
    amount DECIMAL(15, 2) NOT NULL,
    income_date DATE NOT NULL,
    payment_method VARCHAR(50), -- cash, bank_transfer, credit_card, etc.
    status VARCHAR(50) DEFAULT 'confirmed', -- confirmed, pending, cancelled
    notes TEXT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_incomes_tenant_id ON incomes(tenant_id);
CREATE INDEX idx_incomes_income_date ON incomes(income_date);
CREATE INDEX idx_incomes_category ON incomes(category);

-- 支出表
CREATE TABLE IF NOT EXISTS expenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    expense_type VARCHAR(50) NOT NULL, -- purchase, salary, rent, utility, other
    reference_id UUID, -- 關聯的採購單ID等
    reference_type VARCHAR(50), -- purchase_order, etc.
    category VARCHAR(100) NOT NULL, -- 支出類別
    description TEXT,
    amount DECIMAL(15, 2) NOT NULL,
    expense_date DATE NOT NULL,
    payment_method VARCHAR(50), -- cash, bank_transfer, credit_card, etc.
    vendor VARCHAR(255), -- 供應商/收款方
    status VARCHAR(50) DEFAULT 'confirmed', -- confirmed, pending, cancelled
    notes TEXT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_expenses_tenant_id ON expenses(tenant_id);
CREATE INDEX idx_expenses_expense_date ON expenses(expense_date);
CREATE INDEX idx_expenses_category ON expenses(category);

-- ============================================
-- 客服模組
-- ============================================

-- 客服通訊表
CREATE TABLE IF NOT EXISTS support_communications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,
    staff_id UUID REFERENCES users(id) ON DELETE SET NULL,
    communication_type VARCHAR(50) NOT NULL, -- phone, email, chat, ticket, etc.
    subject VARCHAR(255),
    content TEXT NOT NULL,
    direction VARCHAR(20) NOT NULL, -- inbound, outbound
    status VARCHAR(50) DEFAULT 'open', -- open, in_progress, resolved, closed
    priority VARCHAR(20) DEFAULT 'normal', -- low, normal, high, urgent
    resolved_at TIMESTAMP,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 推廣發送表
CREATE TABLE IF NOT EXISTS promotions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    promotion_type VARCHAR(50) NOT NULL, -- sms, email, push, in_app, etc.
    target_audience JSONB DEFAULT '{}', -- 目標受眾條件
    scheduled_at TIMESTAMP,
    sent_at TIMESTAMP,
    total_recipients INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    fail_count INTEGER DEFAULT 0,
    status VARCHAR(50) DEFAULT 'draft', -- draft, scheduled, sending, sent, cancelled
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- POS模組
-- ============================================

-- POS銷售表
CREATE TABLE IF NOT EXISTS pos_sales (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
    staff_id UUID REFERENCES users(id) ON DELETE SET NULL,
    sale_number VARCHAR(100) UNIQUE,
    sale_date TIMESTAMP NOT NULL,
    subtotal DECIMAL(18, 2) DEFAULT 0.00,
    discount_amount DECIMAL(18, 2) DEFAULT 0.00,
    tax_amount DECIMAL(18, 2) DEFAULT 0.00,
    total_amount DECIMAL(18, 2) DEFAULT 0.00,
    status VARCHAR(50) DEFAULT 'completed', -- pending, completed, refunded, cancelled
    notes TEXT,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- POS銷售明細表
CREATE TABLE IF NOT EXISTS pos_sale_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pos_sale_id UUID REFERENCES pos_sales(id) ON DELETE CASCADE,
    product_id UUID REFERENCES products(id) ON DELETE SET NULL,
    service_id UUID REFERENCES services(id) ON DELETE SET NULL,
    item_name VARCHAR(255) NOT NULL,
    quantity INTEGER NOT NULL,
    unit_price DECIMAL(18, 2) DEFAULT 0.00,
    discount_amount DECIMAL(18, 2) DEFAULT 0.00,
    tax_rate DECIMAL(5, 2) DEFAULT 0.00,
    total_amount DECIMAL(18, 2) DEFAULT 0.00,
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- POS支付表
CREATE TABLE IF NOT EXISTS pos_payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pos_sale_id UUID REFERENCES pos_sales(id) ON DELETE CASCADE,
    payment_method VARCHAR(50) NOT NULL, -- cash, card, mobile, voucher, etc.
    amount DECIMAL(18, 2) NOT NULL,
    currency_id UUID REFERENCES currencies(id) ON DELETE SET NULL,
    exchange_rate DECIMAL(18, 6) DEFAULT 1.0,
    reference_number VARCHAR(255),
    status VARCHAR(50) DEFAULT 'completed', -- pending, completed, refunded, failed
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- 索引
-- ============================================

-- 常用查詢索引
CREATE INDEX IF NOT EXISTS idx_customers_tenant ON customers(tenant_id);
CREATE INDEX IF NOT EXISTS idx_customers_member_level ON customers(member_level_id);
CREATE INDEX IF NOT EXISTS idx_products_tenant ON products(tenant_id);
CREATE INDEX IF NOT EXISTS idx_products_type ON products(product_type_id);
CREATE INDEX IF NOT EXISTS idx_products_brand ON products(brand_id);
CREATE INDEX IF NOT EXISTS idx_orders_tenant ON orders(tenant_id);
CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_appointments_tenant ON appointments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_appointments_customer ON appointments(customer_id);
CREATE INDEX IF NOT EXISTS idx_appointments_date ON appointments(appointment_date);
CREATE INDEX IF NOT EXISTS idx_messages_tenant ON messages(tenant_id);
CREATE INDEX IF NOT EXISTS idx_messages_to_user ON messages(to_user_id);
CREATE INDEX IF NOT EXISTS idx_calendars_user ON calendars(user_id);
CREATE INDEX IF NOT EXISTS idx_reminders_user ON reminders(user_id);
CREATE INDEX IF NOT EXISTS idx_points_customer ON points(customer_id);
CREATE INDEX IF NOT EXISTS idx_pos_sales_tenant ON pos_sales(tenant_id);
CREATE INDEX IF NOT EXISTS idx_pos_sales_date ON pos_sales(sale_date);

-- ============================================
-- 優惠券和積分設置
-- ============================================

-- 優惠券表
CREATE TABLE IF NOT EXISTS coupons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    coupon_type VARCHAR(50) NOT NULL, -- percentage, fixed_amount, free_shipping
    discount_value DECIMAL(18, 2) DEFAULT 0.00,
    min_purchase DECIMAL(18, 2) DEFAULT 0.00,
    max_discount DECIMAL(18, 2),
    valid_from DATE NOT NULL,
    valid_to DATE NOT NULL,
    usage_limit INTEGER,
    used_count INTEGER DEFAULT 0,
    customer_limit INTEGER,
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, code)
);

-- 積分設置表（每個租戶只有一條記錄）
CREATE TABLE IF NOT EXISTS point_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    points_per_dollar DECIMAL(10, 2) DEFAULT 1.00, -- 每消費1元獲得多少積分
    dollar_per_point DECIMAL(10, 4) DEFAULT 0.01, -- 每1積分等於多少現金
    min_points_to_use INTEGER DEFAULT 0, -- 最低使用積分
    max_points_percent DECIMAL(5, 2), -- 積分最多可抵扣訂單金額的百分比
    status VARCHAR(20) DEFAULT 'active',
    extra_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id)
);

-- 更新 orders 表，添加優惠券和積分字段
ALTER TABLE orders ADD COLUMN IF NOT EXISTS coupon_id UUID REFERENCES coupons(id) ON DELETE SET NULL;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS points_used INTEGER DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS points_earned INTEGER DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS points_discount DECIMAL(18, 2) DEFAULT 0.00;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS coupon_discount DECIMAL(18, 2) DEFAULT 0.00;

CREATE INDEX IF NOT EXISTS idx_coupons_tenant ON coupons(tenant_id);
CREATE INDEX IF NOT EXISTS idx_coupons_code ON coupons(tenant_id, code);
CREATE INDEX IF NOT EXISTS idx_point_settings_tenant ON point_settings(tenant_id);

-- 更新 products 表，添加 SKU 和 barcode 字段
ALTER TABLE products ADD COLUMN IF NOT EXISTS sku VARCHAR(100);
ALTER TABLE products ADD COLUMN IF NOT EXISTS barcode VARCHAR(100);
CREATE INDEX IF NOT EXISTS idx_products_sku ON products(tenant_id, sku);
CREATE INDEX IF NOT EXISTS idx_products_barcode ON products(tenant_id, barcode);

