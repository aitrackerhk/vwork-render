-- 創建 blogs 表（博客文章）
CREATE TABLE IF NOT EXISTS blogs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL,
    content TEXT, -- HTML 內容
    excerpt TEXT, -- 摘要
    featured_image VARCHAR(500), -- 特色圖片 URL
    author_id UUID REFERENCES users(id),
    status VARCHAR(50) NOT NULL DEFAULT 'draft', -- draft, published, archived
    published_at TIMESTAMP WITH TIME ZONE,
    category VARCHAR(100), -- 分類
    tags JSONB DEFAULT '[]'::jsonb, -- 標籤數組
    view_count INTEGER DEFAULT 0,
    seo_title VARCHAR(255),
    seo_description TEXT,
    seo_keywords VARCHAR(500),
    created_by UUID REFERENCES users(id),
    updated_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    extra_fields JSONB DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_blogs_tenant ON blogs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_blogs_slug ON blogs(tenant_id, slug);
CREATE INDEX IF NOT EXISTS idx_blogs_status ON blogs(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_blogs_published_at ON blogs(tenant_id, published_at) WHERE status = 'published';
CREATE UNIQUE INDEX IF NOT EXISTS idx_blogs_tenant_slug_unique ON blogs(tenant_id, slug);

