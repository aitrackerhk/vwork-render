package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetWebsiteTheme 獲取當前租戶的網站主題設置
func GetWebsiteTheme(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Preload("IndustryTemplate").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 獲取行業模板代碼
	var industryTemplateCode string
	if tenant.IndustryTemplate != nil {
		industryTemplateCode = tenant.IndustryTemplate.Code
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"website_theme":          tenant.WebsiteTheme,
			"website_type":           tenant.WebsiteType,
			"website_enabled":        tenant.WebsiteEnabled,
			"industry_template_code": industryTemplateCode,
		},
	})
}

// UpdateWebsiteTheme 更新當前租戶的網站主題設置
func UpdateWebsiteTheme(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var req struct {
		WebsiteTheme   *string `json:"website_theme"`
		WebsiteType    *string `json:"website_type"`    // 'ecommerce', 'general'
		WebsiteEnabled *bool   `json:"website_enabled"`
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

	updates := make(map[string]interface{})
	if req.WebsiteTheme != nil {
		updates["website_theme"] = *req.WebsiteTheme
	}
	if req.WebsiteType != nil {
		updates["website_type"] = *req.WebsiteType
	}
	if req.WebsiteEnabled != nil {
		updates["website_enabled"] = *req.WebsiteEnabled
	}

	if err := database.DB.Model(&tenant).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update website theme",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Website theme updated successfully",
		"data": fiber.Map{
			"website_theme":   tenant.WebsiteTheme,
			"website_type":    tenant.WebsiteType,
			"website_enabled": tenant.WebsiteEnabled,
		},
	})
}

// ResetWebsitePages 根據當前行業模板和網站類型重新建立網站頁面
func ResetWebsitePages(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	requestLang := strings.TrimSpace(c.Query("lang"))

	var tenant models.Tenant
	if err := database.DB.Preload("IndustryTemplate").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 刪除所有現有頁面和組件
	if err := tx.Where("tenant_id = ?", tenantID).Delete(&models.PageComponent{}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to delete existing components",
		})
	}
	if err := tx.Where("tenant_id = ?", tenantID).Delete(&models.Page{}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to delete existing pages",
		})
	}

	// 根據行業模板和網站類型創建頁面
	var createErr error
	industryCode := ""
	if tenant.IndustryTemplate != nil {
		industryCode = tenant.IndustryTemplate.Code
	}

	websiteType := ""
	if tenant.WebsiteType != nil {
		websiteType = *tenant.WebsiteType
	}

	// 優先根據行業模板決定頁面類型
	switch industryCode {
	case "dining":
		// 餐飲業：創建餐飲專用頁面（包含菜單）
		createErr = createDefaultDiningPagesHandler(tx, tenantID, requestLang)
	case "service":
		// 服務業：創建服務專用頁面（包含預約）
		createErr = createDefaultServicePagesHandler(tx, tenantID, requestLang)
	default:
		// 其他行業：根據網站類型決定
		if websiteType == "ecommerce" {
			createErr = createDefaultEcommercePagesHandler(tx, tenantID, requestLang)
		} else {
			createErr = createDefaultGeneralPagesHandler(tx, tenantID, requestLang)
		}
	}

	if createErr != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create pages: " + createErr.Error(),
		})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to commit transaction",
		})
	}

	return c.JSON(fiber.Map{
		"message":               "Website pages reset successfully",
		"industry_template":     industryCode,
		"website_type":          websiteType,
	})
}