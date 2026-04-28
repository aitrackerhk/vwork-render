-- Update missing featured_image for zh-CN and en blog posts
-- Copy the featured_image from the zh (Traditional Chinese) version of each article

UPDATE platform_blogs
SET featured_image = sub.zh_image, updated_at = NOW()
FROM (
    SELECT slug, featured_image AS zh_image
    FROM platform_blogs
    WHERE lang = 'zh' AND featured_image IS NOT NULL
) sub
WHERE platform_blogs.slug = sub.slug
  AND platform_blogs.lang IN ('zh-CN', 'en')
  AND platform_blogs.featured_image IS NULL;
