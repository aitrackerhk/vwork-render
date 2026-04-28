-- Platform-level blog posts (independent of tenants, managed via vworkadmin)
CREATE TABLE IF NOT EXISTS platform_blogs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(500) NOT NULL,
    slug VARCHAR(500) NOT NULL UNIQUE,
    content TEXT,
    excerpt TEXT,
    featured_image VARCHAR(1000),
    author VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    published_at TIMESTAMP WITH TIME ZONE,
    category VARCHAR(200),
    tags JSONB DEFAULT '[]'::jsonb,
    view_count INTEGER DEFAULT 0,
    seo_title VARCHAR(500),
    seo_description TEXT,
    seo_keywords VARCHAR(500),
    lang VARCHAR(10) DEFAULT 'zh',
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_platform_blogs_slug ON platform_blogs(slug);
CREATE INDEX IF NOT EXISTS idx_platform_blogs_status ON platform_blogs(status);
CREATE INDEX IF NOT EXISTS idx_platform_blogs_published_at ON platform_blogs(published_at) WHERE status = 'published';
CREATE INDEX IF NOT EXISTS idx_platform_blogs_category ON platform_blogs(category);
CREATE INDEX IF NOT EXISTS idx_platform_blogs_lang ON platform_blogs(lang);
