package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetWebsiteSettings 獲取網站設定
func GetWebsiteSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 從 extra_fields 中獲取設定
	allowedCountries := []string{}
	if countries, ok := tenant.ExtraFields["allowed_countries"].([]interface{}); ok {
		for _, c := range countries {
			if country, ok := c.(string); ok {
				allowedCountries = append(allowedCountries, country)
			}
		}
	}

	defaultLanguage := ""
	if lang, ok := tenant.ExtraFields["default_language"].(string); ok {
		defaultLanguage = lang
	} else {
		// 如果沒有設定，從當前語言設定獲取（從localStorage或cookie）
		// 這裡可以從請求中獲取，暫時使用空字符串
		defaultLanguage = ""
	}

	publicChatEnabled := true
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

	websiteIcon := ""
	if icon, ok := tenant.ExtraFields["website_icon"].(string); ok {
		websiteIcon = icon
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"allowed_countries": allowedCountries,
			"default_language":  defaultLanguage,
			"public_chat_enabled": publicChatEnabled,
			"website_icon":       websiteIcon,
		},
	})
}

// UpdateWebsiteSettings 更新網站設定
func UpdateWebsiteSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var req struct {
		AllowedCountries []string `json:"allowed_countries"`
		DefaultLanguage  *string  `json:"default_language"`
		PublicChatEnabled *bool   `json:"public_chat_enabled"`
		WebsiteIcon       *string `json:"website_icon"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 更新 extra_fields
	if tenant.ExtraFields == nil {
		tenant.ExtraFields = models.JSONB{}
	}

	if req.AllowedCountries != nil {
		tenant.ExtraFields["allowed_countries"] = req.AllowedCountries
	}
	if req.DefaultLanguage != nil {
		tenant.ExtraFields["default_language"] = *req.DefaultLanguage
	}
	if req.PublicChatEnabled != nil {
		tenant.ExtraFields["public_chat_enabled"] = *req.PublicChatEnabled
	}
	if req.WebsiteIcon != nil {
		if *req.WebsiteIcon == "" {
			delete(tenant.ExtraFields, "website_icon")
		} else {
			tenant.ExtraFields["website_icon"] = *req.WebsiteIcon
		}
	}

	if err := database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update website settings",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Website settings updated successfully",
		"data": fiber.Map{
			"allowed_countries": req.AllowedCountries,
			"default_language":  req.DefaultLanguage,
			"public_chat_enabled": req.PublicChatEnabled,
		},
	})
}
