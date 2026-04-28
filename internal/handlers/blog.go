package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetBlogs 獲取博客列表
func GetBlogs(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var blogs []models.Blog

	query := database.DB.Where("tenant_id = ?", tenantID)

	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where("title ILIKE ? OR slug ILIKE ? OR excerpt ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 分類過濾
	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	database.DB.Model(&models.Blog{}).Where("tenant_id = ?", tenantID).Count(&total)

	if err := query.
		Preload("Author").
		Offset(offset).Limit(limit).
		Order("created_at DESC").
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

// GetBlog 獲取單個博客
func GetBlog(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	blogID := c.Params("id")

	var blog models.Blog
	if err := database.DB.
		Where("tenant_id = ? AND id = ?", tenantID, blogID).
		Preload("Author").
		First(&blog).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Blog not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch blog: %v", err)})
	}

	return c.JSON(blog)
}

// CreateBlog 創建博客
func CreateBlog(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Title         string                 `json:"title"`
		Slug          string                 `json:"slug"`
		Content       *string                `json:"content,omitempty"`
		Excerpt       *string                `json:"excerpt,omitempty"`
		FeaturedImage *string                `json:"featured_image,omitempty"`
		AuthorID      *uuid.UUID            `json:"author_id,omitempty"`
		Status        string                 `json:"status"`
		PublishedAt   *time.Time             `json:"published_at,omitempty"`
		Category      *string                `json:"category,omitempty"`
		Tags          []interface{}          `json:"tags,omitempty"`
		SEOTitle      *string                `json:"seo_title,omitempty"`
		SEODescription *string               `json:"seo_description,omitempty"`
		SEOKeywords   *string                `json:"seo_keywords,omitempty"`
		ExtraFields   map[string]interface{} `json:"extra_fields,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// 生成 slug（如果未提供）
	if req.Slug == "" {
		req.Slug = strings.ToLower(strings.ReplaceAll(req.Title, " ", "-"))
	}

	// 確保 slug 唯一
	slug := req.Slug
	counter := 1
	for {
		var existing models.Blog
		if err := database.DB.Where("tenant_id = ? AND slug = ?", tenantID, slug).First(&existing).Error; err == gorm.ErrRecordNotFound {
			break
		}
		slug = fmt.Sprintf("%s-%d", req.Slug, counter)
		counter++
	}

	blog := models.Blog{
		TenantID:      tenantID,
		Title:          req.Title,
		Slug:           slug,
		Content:       req.Content,
		Excerpt:        req.Excerpt,
		FeaturedImage:  req.FeaturedImage,
		AuthorID:       req.AuthorID,
		Status:         req.Status,
		PublishedAt:    req.PublishedAt,
		Category:      req.Category,
		SEOTitle:       req.SEOTitle,
		SEODescription: req.SEODescription,
		SEOKeywords:    req.SEOKeywords,
		CreatedBy:      &userID,
		UpdatedBy:      &userID,
	}

	if req.Tags != nil {
		blog.Tags = models.JSONB(map[string]interface{}{"_data": req.Tags})
	}

	if req.ExtraFields != nil {
		blog.ExtraFields = models.JSONB(req.ExtraFields)
	}

	// 如果狀態是 published 且未設置發布時間，設置為現在
	if blog.Status == "published" && blog.PublishedAt == nil {
		now := time.Now()
		blog.PublishedAt = &now
	}

	if err := database.DB.Create(&blog).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create blog: %v", err)})
	}

	return c.Status(201).JSON(blog)
}

// UpdateBlog 更新博客
func UpdateBlog(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	blogID := c.Params("id")

	var blog models.Blog
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, blogID).First(&blog).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Blog not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch blog: %v", err)})
	}

	var req struct {
		Title         *string                `json:"title,omitempty"`
		Slug          *string                `json:"slug,omitempty"`
		Content       *string                `json:"content,omitempty"`
		Excerpt       *string                `json:"excerpt,omitempty"`
		FeaturedImage *string                `json:"featured_image,omitempty"`
		AuthorID      *uuid.UUID             `json:"author_id,omitempty"`
		Status        *string                `json:"status,omitempty"`
		PublishedAt   *time.Time             `json:"published_at,omitempty"`
		Category      *string                `json:"category,omitempty"`
		Tags          []interface{}           `json:"tags,omitempty"`
		SEOTitle      *string                `json:"seo_title,omitempty"`
		SEODescription *string               `json:"seo_description,omitempty"`
		SEOKeywords   *string                `json:"seo_keywords,omitempty"`
		ExtraFields   map[string]interface{} `json:"extra_fields,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	updates := make(map[string]interface{})
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Slug != nil {
		updates["slug"] = *req.Slug
	}
	if req.Content != nil {
		updates["content"] = *req.Content
	}
	if req.Excerpt != nil {
		updates["excerpt"] = *req.Excerpt
	}
	if req.FeaturedImage != nil {
		updates["featured_image"] = *req.FeaturedImage
	}
	if req.AuthorID != nil {
		updates["author_id"] = *req.AuthorID
	}
	if req.Status != nil {
		updates["status"] = *req.Status
		// 如果狀態改為 published 且未設置發布時間，設置為現在
		if *req.Status == "published" && blog.PublishedAt == nil {
			now := time.Now()
			updates["published_at"] = now
		}
	}
	if req.PublishedAt != nil {
		updates["published_at"] = *req.PublishedAt
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.Tags != nil {
		updates["tags"] = models.JSONB(map[string]interface{}{"_data": req.Tags})
	}
	if req.SEOTitle != nil {
		updates["seo_title"] = *req.SEOTitle
	}
	if req.SEODescription != nil {
		updates["seo_description"] = *req.SEODescription
	}
	if req.SEOKeywords != nil {
		updates["seo_keywords"] = *req.SEOKeywords
	}
	if req.ExtraFields != nil {
		updates["extra_fields"] = models.JSONB(req.ExtraFields)
	}

	updates["updated_by"] = userID

	if err := database.DB.Model(&blog).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update blog: %v", err)})
	}

	// 重新載入
	database.DB.Where("id = ?", blogID).Preload("Author").First(&blog)

	return c.JSON(blog)
}

// DeleteBlog 刪除博客
func DeleteBlog(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	blogID := c.Params("id")

	var blog models.Blog
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, blogID).First(&blog).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Blog not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch blog: %v", err)})
	}

	if err := database.DB.Delete(&blog).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete blog: %v", err)})
	}

	return c.JSON(fiber.Map{"message": "Blog deleted successfully"})
}

