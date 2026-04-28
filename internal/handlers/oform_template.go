package handlers

import (
	"math"
	"nwork/internal/database"
	"nwork/internal/models"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// GetOformTemplates serves vOffice desktop app template listing
// Compatible with OnlyOffice oforms API format: /dashboard/api/oforms?populate=*&locale=en&pagination[page]=1
func GetOformTemplates(c *fiber.Ctx) error {
	locale := c.Query("locale", "en")
	page := c.QueryInt("pagination[page]", 1)
	pageSize := c.QueryInt("pagination[pageSize]", 25)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}

	offset := (page - 1) * pageSize

	var templates []models.OformTemplate
	var total int64

	// Try exact locale match first, then fallback to base language
	locales := buildLocaleFallbacks(locale)

	query := database.DB.Where("is_active = true AND locale IN ?", locales)
	query.Model(&models.OformTemplate{}).Count(&total)

	if err := query.Order("sort_order ASC, id ASC").
		Offset(offset).Limit(pageSize).
		Find(&templates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch templates"})
	}

	// Convert to OnlyOffice-compatible oforms response format
	data := make([]fiber.Map, 0, len(templates))
	for _, t := range templates {
		data = append(data, toOformJSON(t))
	}

	pageCount := int(math.Ceil(float64(total) / float64(pageSize)))

	return c.JSON(fiber.Map{
		"data": data,
		"meta": fiber.Map{
			"pagination": fiber.Map{
				"page":      page,
				"pageSize":  pageSize,
				"pageCount": pageCount,
				"total":     total,
			},
		},
	})
}

// toOformJSON converts an OformTemplate to the nested JSON structure
// expected by paneltemplates.js _on_add_cloud_templates
func toOformJSON(t models.OformTemplate) fiber.Map {
	// Build file_oform data
	var fileOformData []fiber.Map
	if t.FileURL != "" {
		fileOformData = []fiber.Map{
			{
				"attributes": fiber.Map{
					"url":  t.FileURL,
					"size": t.FileSize,
				},
			},
		}
	}

	// Build card_prewiew (note: OnlyOffice original has typo "prewiew")
	var cardPreview interface{}
	if t.PreviewURL != "" {
		cardPreview = fiber.Map{
			"data": fiber.Map{
				"attributes": fiber.Map{
					"url": t.PreviewURL,
				},
			},
		}
	}

	// Build template_image with thumbnail
	var templateImage interface{}
	if t.ThumbnailURL != "" {
		templateImage = fiber.Map{
			"data": fiber.Map{
				"attributes": fiber.Map{
					"formats": fiber.Map{
						"thumbnail": fiber.Map{
							"url": t.ThumbnailURL,
						},
					},
				},
			},
		}
	}

	return fiber.Map{
		"id": t.ID,
		"attributes": fiber.Map{
			"name_form":      t.NameForm,
			"template_desc":  t.TemplateDesc,
			"card_prewiew":   cardPreview,
			"template_image": templateImage,
			"file_oform": fiber.Map{
				"data": fileOformData,
			},
			"form_exts": fiber.Map{
				"data": []fiber.Map{
					{
						"attributes": fiber.Map{
							"ext": t.FileExt,
						},
					},
				},
			},
		},
	}
}

// buildLocaleFallbacks returns locale variants for DB query
// e.g. "zh-TW" -> ["zh-TW", "zh", "en"]
func buildLocaleFallbacks(locale string) []string {
	locales := []string{locale}

	// Add base language if locale has region (e.g. zh-TW -> zh)
	if parts := strings.SplitN(locale, "-", 2); len(parts) == 2 {
		locales = append(locales, parts[0])
	}
	// Also try underscore variant (zh_TW)
	if strings.Contains(locale, "-") {
		locales = append(locales, strings.ReplaceAll(locale, "-", "_"))
	}

	// Always fallback to English
	if locale != "en" {
		locales = append(locales, "en")
	}

	return locales
}

// === Admin APIs for managing oform templates ===

// AdminGetOformTemplates lists all templates (including inactive) for admin management
func AdminGetOformTemplates(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	offset := (page - 1) * limit

	var templates []models.OformTemplate
	var total int64

	query := database.DB.Model(&models.OformTemplate{})

	if locale := c.Query("locale"); locale != "" {
		query = query.Where("locale = ?", locale)
	}
	if search := c.Query("search"); search != "" {
		query = query.Where("name_form ILIKE ?", "%"+search+"%")
	}

	query.Count(&total)

	if err := query.Order("sort_order ASC, id ASC").
		Offset(offset).Limit(limit).
		Find(&templates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch templates"})
	}

	return c.JSON(fiber.Map{
		"data":  templates,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// AdminCreateOformTemplate creates a new template
func AdminCreateOformTemplate(c *fiber.Ctx) error {
	var t models.OformTemplate
	if err := c.BodyParser(&t); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if t.NameForm == "" || t.FileURL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name_form and file_url are required"})
	}
	if t.Locale == "" {
		t.Locale = "en"
	}
	if t.FileExt == "" {
		t.FileExt = "pdf"
	}
	t.IsActive = true

	if err := database.DB.Create(&t).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create template: " + err.Error()})
	}

	return c.Status(201).JSON(t)
}

// AdminUpdateOformTemplate updates an existing template
func AdminUpdateOformTemplate(c *fiber.Ctx) error {
	id := c.Params("id")

	var t models.OformTemplate
	if err := database.DB.First(&t, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Template not found"})
	}

	if err := c.BodyParser(&t); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if err := database.DB.Save(&t).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update template: " + err.Error()})
	}

	return c.JSON(t)
}

// AdminDeleteOformTemplate deletes a template
func AdminDeleteOformTemplate(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := database.DB.Delete(&models.OformTemplate{}, id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete template: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Template deleted"})
}
