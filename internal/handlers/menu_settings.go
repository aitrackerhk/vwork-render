package handlers

import (
	"encoding/json"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// 用戶菜單設定 API
// ============================================

// GetUserMenuSettings 獲取當前用戶的菜單設定
func GetUserMenuSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if tenantID == uuid.Nil || userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var settings models.UserMenuSettings
	err := database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID).First(&settings).Error
	if err != nil {
		// 如果不存在，返回空陣列
		return c.JSON(fiber.Map{
			"menu_config": fiber.Map{"items": []interface{}{}},
		})
	}

	return c.JSON(fiber.Map{
		"id":          settings.ID,
		"menu_config": settings.MenuConfig,
	})
}

// SaveUserMenuSettings 保存用戶菜單設定
func SaveUserMenuSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if tenantID == uuid.Nil || userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// 解析請求體
	var input struct {
		MenuConfig    json.RawMessage `json:"menu_config"`
		HiddenSubkeys []string        `json:"hidden_subkeys"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// 嘗試作為陣列解析
	var menuItems []map[string]interface{}
	if err := json.Unmarshal(input.MenuConfig, &menuItems); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid menu_config format: expected array"})
	}

	// 將陣列轉換為 JSONB 格式（包裝在 items 中，包含 hidden_subkeys）
	menuConfigJSONB := models.JSONB{"items": menuItems}
	if len(input.HiddenSubkeys) > 0 {
		menuConfigJSONB["hidden_subkeys"] = input.HiddenSubkeys
	}

	// 查找或創建設定
	var settings models.UserMenuSettings
	err := database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID).First(&settings).Error
	if err != nil {
		// 創建新記錄
		settings = models.UserMenuSettings{
			TenantID:   tenantID,
			UserID:     userID,
			MenuConfig: menuConfigJSONB,
		}
		if err := database.DB.Create(&settings).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save menu settings"})
		}
	} else {
		// 更新現有記錄
		settings.MenuConfig = menuConfigJSONB
		if err := database.DB.Save(&settings).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update menu settings"})
		}
	}

	return c.JSON(fiber.Map{
		"success":     true,
		"id":          settings.ID,
		"menu_config": settings.MenuConfig,
	})
}
