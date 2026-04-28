package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetPosSettings 獲取 POS 設定
func GetPosSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var settings models.PosSettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error; err != nil {
		// 如果不存在，返回默認值
		return c.JSON(models.PosSettings{
			TenantID:       tenantID,
			DepositPercent: 0,
			DepositFixed:   0,
		})
	}

	return c.JSON(settings)
}

// CreateOrUpdatePosSettings 創建或更新 POS 設定
func CreateOrUpdatePosSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		DepositPercent     float64 `json:"deposit_percent"`
		DepositFixed       float64 `json:"deposit_fixed"`
		AllowGuestCheckout *bool   `json:"allow_guest_checkout"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 驗證百分比範圍
	if req.DepositPercent < 0 || req.DepositPercent > 100 {
		return c.Status(400).JSON(fiber.Map{"error": "Deposit percent must be between 0 and 100"})
	}

	// 驗證固定金額
	if req.DepositFixed < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Deposit fixed amount cannot be negative"})
	}

	var settings models.PosSettings
	err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error

	allowGuest := false
	if req.AllowGuestCheckout != nil {
		allowGuest = *req.AllowGuestCheckout
	}

	if err != nil {
		// 創建新設定
		settings = models.PosSettings{
			TenantID:           tenantID,
			DepositPercent:     req.DepositPercent,
			DepositFixed:       req.DepositFixed,
			AllowGuestCheckout: allowGuest,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		if err := database.DB.Create(&settings).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create POS settings"})
		}
	} else {
		// 更新現有設定
		settings.DepositPercent = req.DepositPercent
		settings.DepositFixed = req.DepositFixed
		if req.AllowGuestCheckout != nil {
			settings.AllowGuestCheckout = allowGuest
		}
		settings.UpdatedAt = time.Now()
		if err := database.DB.Save(&settings).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update POS settings"})
		}
	}

	return c.JSON(settings)
}

