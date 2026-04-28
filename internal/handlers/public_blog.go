package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/themes"
)

// PublicGetBlogs returns published blogs for a tenant (public, no auth required).
// Used by the blog-list component on the public website.
func PublicGetBlogs(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")

	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	query := database.DB.
		Where("tenant_id = ? AND status = ? AND trashed_at IS NULL", tenant.ID, "published")

	// Category filter
	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}

	// Pagination
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if limit > 50 {
		limit = 50 // cap for public endpoint
	}
	offset := (page - 1) * limit

	var total int64
	database.DB.Model(&models.Blog{}).
		Where("tenant_id = ? AND status = ? AND trashed_at IS NULL", tenant.ID, "published").
		Count(&total)

	var blogs []models.Blog
	if err := query.
		Offset(offset).Limit(limit).
		Order("published_at DESC, created_at DESC").
		Find(&blogs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch blogs: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  blogs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// PublicGetBlog returns a single published blog by slug for a tenant (public, no auth required).
func PublicGetBlog(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	slug := c.Params("slug")

	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var blog models.Blog
	if err := database.DB.
		Where("tenant_id = ? AND slug = ? AND status = ? AND trashed_at IS NULL", tenant.ID, slug, "published").
		First(&blog).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Blog not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch blog: %v", err)})
	}

	// Increment view count (fire-and-forget)
	database.DB.Model(&blog).UpdateColumn("view_count", gorm.Expr("view_count + 1"))

	return c.JSON(blog)
}

// RenderPublicBlogPost renders a server-side blog post page with full SEO support.
// Route: GET /co/:subdomain/blog/:slug/
func RenderPublicBlogPost(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	slug := c.Params("slug")

	// 1. Look up tenant
	var tenant models.Tenant
	if err := database.DB.Where("subdomain = ? AND status = ?", subdomain, "active").First(&tenant).Error; err != nil {
		return c.Render("pages/public_page_not_found", fiber.Map{
			"Message": "暫時沒有此頁",
		}, "layouts/embed_layout")
	}

	// 2. Check website enabled
	if !tenant.WebsiteEnabled {
		return c.Render("pages/public_page_not_found", fiber.Map{
			"Message": "網站暫時關閉",
		}, "layouts/embed_layout")
	}

	// 3. Query blog
	var blog models.Blog
	if err := database.DB.
		Where("tenant_id = ? AND slug = ? AND status = ? AND trashed_at IS NULL", tenant.ID, slug, "published").
		First(&blog).Error; err != nil {
		return c.Render("pages/public_page_not_found", fiber.Map{
			"Message": "暫時沒有此頁",
		}, "layouts/embed_layout")
	}

	// 4. Increment view count (fire-and-forget, GET only)
	if c.Method() == fiber.MethodGet {
		database.DB.Model(&blog).UpdateColumn("view_count", gorm.Expr("view_count + 1"))
	}

	// 5. Load homepage for nav/footer components (reuse site chrome)
	var homepage models.Page
	database.DB.
		Where("tenant_id = ? AND is_homepage = ? AND status IN ? AND trashed_at IS NULL", tenant.ID, true, []string{"draft", "published"}).
		Preload("Components", func(db *gorm.DB) *gorm.DB {
			return db.Where("is_active = ? AND component_type IN ?", true, []string{"header", "nav", "footer"}).Order("sort_order ASC")
		}).
		First(&homepage)

	// Resolve linked blocks and links for the chrome components
	if homepage.ID != [16]byte{} {
		resolveLinkedBlocks(tenant.ID, &homepage)
		resolveComponentLinks(&homepage, subdomain)
	}

	// 6. Tenant language
	publicDefaultLang := "zh"
	switch resolveTenantDefaultPageLang(tenant) {
	case "en":
		publicDefaultLang = "en"
	case "zh-hans":
		publicDefaultLang = "zh-CN"
	}

	// 7. Tenant icon
	tenantIcon := "/static/vworkicon.png"
	if tenant.ExtraFields != nil {
		if icon, ok := tenant.ExtraFields["website_icon"].(string); ok && icon != "" {
			tenantIcon = icon
		}
	}

	// 8. Theme
	themeID := "default"
	if tenant.WebsiteTheme != nil && *tenant.WebsiteTheme != "" {
		themeID = *tenant.WebsiteTheme
	}
	themeCSS := themes.GetThemeCSS(themeID)

	// 9. Build SEO data
	blogTitle := blog.Title
	if blog.SEOTitle != nil && strings.TrimSpace(*blog.SEOTitle) != "" {
		blogTitle = strings.TrimSpace(*blog.SEOTitle)
	}
	blogDescription := ""
	if blog.SEODescription != nil {
		blogDescription = strings.TrimSpace(*blog.SEODescription)
	}
	if blogDescription == "" && blog.Excerpt != nil {
		blogDescription = strings.TrimSpace(*blog.Excerpt)
	}
	blogKeywords := ""
	if blog.SEOKeywords != nil {
		blogKeywords = strings.TrimSpace(*blog.SEOKeywords)
	}

	canonicalURL := buildAbsoluteURL(c, fmt.Sprintf("/co/%s/blog/%s/", subdomain, blog.Slug))

	imageURL := tenantIcon
	if blog.FeaturedImage != nil && *blog.FeaturedImage != "" {
		imageURL = *blog.FeaturedImage
	}
	if imageURL != "" && !strings.HasPrefix(imageURL, "http://") && !strings.HasPrefix(imageURL, "https://") {
		imageURL = buildAbsoluteURL(c, imageURL)
	}

	// 10. Build JSON-LD (BlogPosting schema)
	jsonLD := buildBlogPostJSONLD(blog, canonicalURL, imageURL)

	seo := SEOData{
		Title:         blogTitle,
		Description:   blogDescription,
		Keywords:      blogKeywords,
		CanonicalURL:  canonicalURL,
		ImageURL:      imageURL,
		Type:          "article",
		Locale:        "zh_TW",
		JSONLD:        jsonLD,
		ArticleAuthor: tenant.Name,
	}
	if blog.PublishedAt != nil {
		seo.ArticlePublishedTime = blog.PublishedAt.Format(time.RFC3339)
	}

	// 11. Public chat enabled
	publicChatEnabled := true
	if tenant.ExtraFields != nil {
		if v, ok := tenant.ExtraFields["public_chat_enabled"]; ok {
			switch vv := v.(type) {
			case bool:
				publicChatEnabled = vv
			case string:
				s := strings.ToLower(strings.TrimSpace(vv))
				publicChatEnabled = (s == "true" || s == "1" || s == "on")
			case float64:
				publicChatEnabled = int(vv) == 1
			case int:
				publicChatEnabled = vv == 1
			}
		}
	}

	return c.Render("pages/public_blog_post", fiber.Map{
		"Blog":              blog,
		"Page":              homepage, // for nav/footer components
		"SEO":               seo,
		"Subdomain":         subdomain,
		"PublicDefaultLang": publicDefaultLang,
		"PublicChatEnabled": publicChatEnabled,
		"TenantIcon":        tenantIcon,
		"ThemeCSS":          themeCSS,
		"ThemeID":           themeID,
	})
}

// buildBlogPostJSONLD generates a BlogPosting JSON-LD structured data snippet.
func buildBlogPostJSONLD(blog models.Blog, canonicalURL string, imageURL string) template.JS {
	ld := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "BlogPosting",
		"headline": blog.Title,
		"url":      canonicalURL,
		"mainEntityOfPage": map[string]interface{}{
			"@type": "WebPage",
			"@id":   canonicalURL,
		},
	}

	if blog.SEODescription != nil && *blog.SEODescription != "" {
		ld["description"] = *blog.SEODescription
	} else if blog.Excerpt != nil && *blog.Excerpt != "" {
		ld["description"] = *blog.Excerpt
	}

	if imageURL != "" {
		ld["image"] = imageURL
	}

	if blog.PublishedAt != nil {
		ld["datePublished"] = blog.PublishedAt.UTC().Format(time.RFC3339)
	}
	ld["dateModified"] = blog.UpdatedAt.UTC().Format(time.RFC3339)

	if blog.Category != nil && *blog.Category != "" {
		ld["articleSection"] = *blog.Category
	}

	if blog.SEOKeywords != nil && *blog.SEOKeywords != "" {
		ld["keywords"] = *blog.SEOKeywords
	}

	b, err := json.Marshal(ld)
	if err != nil {
		return ""
	}
	return template.JS(b)
}
