-- vOffice oform templates table
-- Stores document templates for the vOffice desktop app Templates panel
-- API: /dashboard/api/oforms (compatible with OnlyOffice oforms API format)

CREATE TABLE IF NOT EXISTS oform_templates (
    id              SERIAL PRIMARY KEY,
    locale          VARCHAR(10) NOT NULL DEFAULT 'en',        -- e.g. en, zh-TW, zh-CN, ja
    name_form       VARCHAR(500) NOT NULL,                     -- display name
    template_desc   TEXT DEFAULT '',                            -- description shown in preview dialog
    file_ext        VARCHAR(20) NOT NULL DEFAULT 'pdf',        -- pdf, docx, xlsx, pptx
    file_url        VARCHAR(1000) NOT NULL,                    -- URL to the actual template file
    file_size       BIGINT DEFAULT 0,                          -- file size in bytes
    thumbnail_url   VARCHAR(1000) DEFAULT '',                  -- small thumbnail for list view
    preview_url     VARCHAR(1000) DEFAULT '',                  -- large preview image
    category        VARCHAR(200) DEFAULT '',                    -- category name for grouping
    sort_order      INT DEFAULT 0,                             -- lower = shown first
    is_active       BOOLEAN DEFAULT true,                      -- soft toggle
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for locale-based queries (the primary query pattern)
CREATE INDEX IF NOT EXISTS idx_oform_templates_locale ON oform_templates(locale);
CREATE INDEX IF NOT EXISTS idx_oform_templates_active ON oform_templates(is_active);
CREATE INDEX IF NOT EXISTS idx_oform_templates_locale_active ON oform_templates(locale, is_active);
