-- Allow same slug across different languages; enforce uniqueness per (lang, slug)

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'platform_blogs_slug_key'
          AND conrelid = 'platform_blogs'::regclass
    ) THEN
        ALTER TABLE platform_blogs DROP CONSTRAINT platform_blogs_slug_key;
    END IF;
END $$;

DROP INDEX IF EXISTS idx_platform_blogs_slug;

CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_blogs_lang_slug
    ON platform_blogs(lang, slug);
