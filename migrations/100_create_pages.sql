-- 頁面管理：pages（公司網站頁面）
CREATE TABLE IF NOT EXISTS pages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL,
    title VARCHAR(255),
    description TEXT,
    topnav_style VARCHAR(50) DEFAULT 'default', -- default, light, dark, transparent
    status VARCHAR(50) NOT NULL DEFAULT 'draft', -- draft, published
    is_homepage BOOLEAN DEFAULT FALSE,
    seo_title VARCHAR(255),
    seo_description TEXT,
    seo_keywords VARCHAR(500),
    created_by UUID REFERENCES users(id),
    updated_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    extra_fields JSONB DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_pages_tenant ON pages(tenant_id);
CREATE INDEX IF NOT EXISTS idx_pages_slug ON pages(slug);
CREATE INDEX IF NOT EXISTS idx_pages_status ON pages(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_pages_tenant_slug_unique ON pages(tenant_id, slug);
CREATE INDEX IF NOT EXISTS idx_pages_is_homepage ON pages(tenant_id, is_homepage) WHERE is_homepage = TRUE;

-- 頁面元件：page_components（存儲頁面中的元件配置）
CREATE TABLE IF NOT EXISTS page_components (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    page_id UUID NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    component_type VARCHAR(100) NOT NULL, -- hero, text, image, button, section, etc.
    component_data JSONB NOT NULL DEFAULT '{}'::jsonb, -- 元件配置數據
    sort_order INT NOT NULL DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    extra_fields JSONB DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_page_components_tenant ON page_components(tenant_id);
CREATE INDEX IF NOT EXISTS idx_page_components_page ON page_components(page_id);
CREATE INDEX IF NOT EXISTS idx_page_components_sort_order ON page_components(page_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_page_components_type ON page_components(component_type);

