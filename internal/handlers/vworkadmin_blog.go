package handlers

import (
	"encoding/base64"
	"fmt"
	"image/jpeg"
	"image/png"
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/models"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func normalizePlatformBlogLang(lang string) string {
	l := strings.TrimSpace(lang)
	if l == "zh" || l == "zh-CN" || l == "en" {
		return l
	}
	ll := strings.ToLower(l)
	if ll == "zh-cn" {
		return "zh-CN"
	}
	if strings.HasPrefix(ll, "zh") {
		return "zh"
	}
	if strings.HasPrefix(ll, "en") {
		return "en"
	}
	return "zh"
}

func resolvePlatformBlogLang(c *fiber.Ctx) string {
	if q := c.Query("lang"); q != "" {
		return normalizePlatformBlogLang(q)
	}
	accept := c.Get("Accept-Language")
	if accept != "" {
		parts := strings.Split(accept, ",")
		if len(parts) > 0 {
			first := strings.TrimSpace(parts[0])
			if i := strings.Index(first, ";"); i >= 0 {
				first = strings.TrimSpace(first[:i])
			}
			if first != "" {
				return normalizePlatformBlogLang(first)
			}
		}
	}
	return "zh"
}

// ── vWorkAdmin: Platform Blog CRUD ──

// VWorkAdminGetBlogs lists all platform blog posts (admin, supports search/filter/pagination)
func VWorkAdminGetBlogs(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset := (page - 1) * limit

	var blogs []models.PlatformBlog
	var total int64

	query := database.DB.Model(&models.PlatformBlog{})

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}
	if lang := c.Query("lang"); lang != "" {
		query = query.Where("lang = ?", lang)
	}
	if search := c.Query("search"); search != "" {
		query = query.Where("title ILIKE ?", "%"+search+"%")
	}

	query.Count(&total)

	if err := query.Order("sort_order ASC, created_at DESC").
		Offset(offset).Limit(limit).
		Find(&blogs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch blogs"})
	}

	return c.JSON(fiber.Map{
		"data":  blogs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// VWorkAdminGetBlog returns a single platform blog post by ID
func VWorkAdminGetBlog(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	var blog models.PlatformBlog
	if err := database.DB.Where("id = ?", id).First(&blog).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Blog not found"})
	}
	return c.JSON(blog)
}

// VWorkAdminCreateBlog creates a new platform blog post
func VWorkAdminCreateBlog(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var blog models.PlatformBlog
	if err := c.BodyParser(&blog); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if blog.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "title is required"})
	}

	// Auto-generate slug if empty
	if blog.Slug == "" {
		blog.Slug = generateSlug(blog.Title)
	}
	blog.Slug = strings.ToLower(strings.TrimSpace(blog.Slug))

	blog.Lang = normalizePlatformBlogLang(blog.Lang)

	// Ensure slug uniqueness within language
	var count int64
	database.DB.Model(&models.PlatformBlog{}).Where("slug = ? AND lang = ?", blog.Slug, blog.Lang).Count(&count)
	if count > 0 {
		blog.Slug = fmt.Sprintf("%s-%d", blog.Slug, time.Now().UnixMilli())
	}

	// Default values
	if blog.Status == "" {
		blog.Status = "draft"
	}
	if blog.Lang == "" {
		blog.Lang = "zh"
	}
	if blog.Author == "" {
		blog.Author = "V-sys"
	}

	// Auto-set published_at when publishing
	if blog.Status == "published" && blog.PublishedAt == nil {
		now := time.Now()
		blog.PublishedAt = &now
	}

	if err := database.DB.Create(&blog).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create blog: " + err.Error()})
	}

	// Auto-generate cover image if no featured_image provided
	if blog.FeaturedImage == nil || *blog.FeaturedImage == "" {
		excerpt := ""
		if blog.Excerpt != nil {
			excerpt = *blog.Excerpt
		}
		if coverURL, err := generateBlogCover(blog.Title, excerpt); err == nil {
			blog.FeaturedImage = &coverURL
			database.DB.Model(&blog).Update("featured_image", coverURL)
		}
		// Non-fatal: if cover generation fails, blog is still created without cover
	}

	return c.Status(201).JSON(blog)
}

// VWorkAdminUpdateBlog updates an existing platform blog post
func VWorkAdminUpdateBlog(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	var blog models.PlatformBlog
	if err := database.DB.Where("id = ?", id).First(&blog).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Blog not found"})
	}

	oldStatus := blog.Status

	if err := c.BodyParser(&blog); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	blog.Lang = normalizePlatformBlogLang(blog.Lang)

	// Auto-set published_at when transitioning to published
	if blog.Status == "published" && oldStatus != "published" && blog.PublishedAt == nil {
		now := time.Now()
		blog.PublishedAt = &now
	}

	if err := database.DB.Save(&blog).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update blog: " + err.Error()})
	}

	return c.JSON(blog)
}

// VWorkAdminDeleteBlog deletes a platform blog post
func VWorkAdminDeleteBlog(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	if err := database.DB.Where("id = ?", id).Delete(&models.PlatformBlog{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete blog: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Blog deleted"})
}

// ── Public: Platform Blog (vWork Official Website) ──

// PublicPlatformBlogList returns published platform blog posts as JSON
func PublicPlatformBlogList(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 12)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 12
	}
	offset := (page - 1) * limit

	var blogs []models.PlatformBlog
	var total int64

	query := database.DB.Model(&models.PlatformBlog{}).
		Where("status = ?", "published")

	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}
	query = query.Where("lang = ?", resolvePlatformBlogLang(c))

	query.Count(&total)

	if err := query.Order("sort_order ASC, published_at DESC").
		Offset(offset).Limit(limit).
		Find(&blogs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch blogs"})
	}

	return c.JSON(fiber.Map{
		"data":  blogs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// RenderPlatformBlogList renders the blog listing page (SSR)
func RenderPlatformBlogList(c *fiber.Ctx) error {
	return c.Render("pages/vwork_blog_list", fiber.Map{
		"PageTitle": "Blog | vWork",
	})
}

// RenderPlatformBlogPost renders a single blog post page (SSR with SEO)
func RenderPlatformBlogPost(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "" {
		return c.Redirect("/vwork-blog")
	}

	lang := resolvePlatformBlogLang(c)

	var blog models.PlatformBlog
	if err := database.DB.Where("slug = ? AND status = ? AND lang = ?", slug, "published", lang).First(&blog).Error; err != nil {
		// Fallback: try to find the same slug in any language
		var fallback models.PlatformBlog
		if fbErr := database.DB.Where("slug = ? AND status = ?", slug, "published").First(&fallback).Error; fbErr == nil {
			// Redirect to the available language version
			return c.Redirect("/vwork-blog/" + slug + "?lang=" + fallback.Lang)
		}
		return c.Status(404).Render("pages/vwork_blog_list", fiber.Map{
			"PageTitle": "Blog | vWork",
			"Error":     "Article not found",
		})
	}

	// Increment view count
	database.DB.Model(&blog).UpdateColumn("view_count", blog.ViewCount+1)

	// SEO fields
	seoTitle := blog.Title + " | vWork Blog"
	if blog.SEOTitle != nil && *blog.SEOTitle != "" {
		seoTitle = *blog.SEOTitle
	}
	seoDesc := ""
	if blog.SEODescription != nil {
		seoDesc = *blog.SEODescription
	} else if blog.Excerpt != nil {
		seoDesc = *blog.Excerpt
	}
	seoKeywords := ""
	if blog.SEOKeywords != nil {
		seoKeywords = *blog.SEOKeywords
	}

	return c.Render("pages/vwork_blog_post", fiber.Map{
		"Blog":        blog,
		"PageTitle":   seoTitle,
		"SEODesc":     seoDesc,
		"SEOKeywords": seoKeywords,
		"CurrentLang": lang,
	})
}

// ── Blog Cover Image Generation (Gemini) ──

// saveBlogCoverFromBase64 decodes a base64 data URL and saves it as a file
// under web/static/blog/. Returns the public URL path.
func saveBlogCoverFromBase64(dataURL string) (string, error) {
	// Parse "data:image/png;base64,AAAA..."
	idx := strings.Index(dataURL, ",")
	if idx < 0 {
		return "", fmt.Errorf("invalid data URL format")
	}
	header := dataURL[:idx] // "data:image/png;base64"
	b64Data := dataURL[idx+1:]

	raw, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Determine extension from mime type
	ext := ".png"
	if strings.Contains(header, "image/jpeg") || strings.Contains(header, "image/jpg") {
		ext = ".jpg"
	}

	// Create directory
	dir := filepath.Join("web", "static", "blog")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create blog image directory: %w", err)
	}

	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	filePath := filepath.Join(dir, filename)

	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		return "", fmt.Errorf("failed to write image file: %w", err)
	}

	// Optionally re-encode as optimized JPEG for smaller file size
	if ext == ".png" {
		// Try to convert PNG to optimized JPEG
		f, err := os.Open(filePath)
		if err == nil {
			defer f.Close()
			img, err := png.Decode(f)
			if err == nil {
				jpgFilename := strings.TrimSuffix(filename, ".png") + ".jpg"
				jpgPath := filepath.Join(dir, jpgFilename)
				jf, err := os.Create(jpgPath)
				if err == nil {
					if err := jpeg.Encode(jf, img, &jpeg.Options{Quality: 90}); err == nil {
						jf.Close()
						os.Remove(filePath)
						filename = jpgFilename
					} else {
						jf.Close()
						os.Remove(jpgPath)
					}
				}
			}
		}
	}

	return fmt.Sprintf("/static/blog/%s", filename), nil
}

// generateBlogCover calls Gemini to generate a cover image for a blog post
// and saves it to disk. Returns the public URL path.
func generateBlogCover(title string, excerpt string) (string, error) {
	cfg := config.Load()
	if cfg.LLM.APIKey == "" {
		return "", fmt.Errorf("Gemini API key not configured")
	}

	// Build a prompt for a professional blog cover
	prompt := fmt.Sprintf(
		"Create a professional, modern blog cover image for an article titled \"%s\". "+
			"The image should be visually appealing, use clean design with subtle tech/business elements. "+
			"NO text or words in the image. Use gradient colors, abstract shapes, or relevant imagery. "+
			"Style: clean, minimal, professional SaaS blog header.",
		title,
	)
	if excerpt != "" && len(excerpt) < 300 {
		prompt += fmt.Sprintf(" The article is about: %s", excerpt)
	}

	// Use 16:9 aspect ratio for blog covers
	dataURLs, err := callGeminiImageGeneration(cfg.LLM.APIKey, cfg.LLM.ImageModel, prompt, "16:9")
	if err != nil {
		return "", fmt.Errorf("Gemini image generation failed: %w", err)
	}
	if len(dataURLs) == 0 {
		return "", fmt.Errorf("Gemini returned no images")
	}

	// Save the first generated image
	coverURL, err := saveBlogCoverFromBase64(dataURLs[0])
	if err != nil {
		return "", fmt.Errorf("failed to save cover image: %w", err)
	}

	return coverURL, nil
}

// VWorkAdminGenerateBlogCover generates (or regenerates) a cover image for an existing blog post.
// POST /api/v1/vworkadmin/platform-blogs/:id/generate-cover
func VWorkAdminGenerateBlogCover(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	var blog models.PlatformBlog
	if err := database.DB.Where("id = ?", id).First(&blog).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Blog not found"})
	}

	excerpt := ""
	if blog.Excerpt != nil {
		excerpt = *blog.Excerpt
	}

	coverURL, err := generateBlogCover(blog.Title, excerpt)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate cover: " + err.Error()})
	}

	// Update the blog's featured image
	blog.FeaturedImage = &coverURL
	if err := database.DB.Model(&blog).Update("featured_image", coverURL).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update blog: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":        "Cover image generated",
		"featured_image": coverURL,
		"blog":           blog,
	})
}

// generateSlug creates a URL-safe slug from a title.
// It replaces spaces with hyphens and removes special characters.
func generateSlug(title string) string {
	slug := strings.ToLower(strings.TrimSpace(title))
	slug = strings.ReplaceAll(slug, " ", "-")
	// Keep alphanumeric, hyphens, and CJK characters
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r > 127 {
			b.WriteRune(r)
		}
	}
	slug = b.String()
	// Remove consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = fmt.Sprintf("post-%d", time.Now().UnixMilli())
	}
	return slug
}
