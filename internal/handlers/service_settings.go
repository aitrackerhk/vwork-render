package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetServiceSettings 取得租戶的服務設定
func GetServiceSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Select("extra_fields").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 預設為三步友善預約模式
	friendlyBooking := parseBoolFromExtra(tenant.ExtraFields["service_friendly_booking"], true)

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"friendly_booking": friendlyBooking,
		},
	})
}

// UpdateServiceSettings 更新租戶的服務設定
func UpdateServiceSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var req struct {
		FriendlyBooking *bool `json:"friendly_booking"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.FriendlyBooking == nil {
		if v := c.FormValue("friendly_booking"); v != "" {
			parsed := parseBoolFromExtra(v, false)
			req.FriendlyBooking = &parsed
		} else if v := c.Query("friendly_booking"); v != "" {
			parsed := parseBoolFromExtra(v, false)
			req.FriendlyBooking = &parsed
		}
	}

	if req.FriendlyBooking == nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "friendly_booking is required",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	if tenant.ExtraFields == nil {
		tenant.ExtraFields = models.JSONB{}
	}
	tenant.ExtraFields["service_friendly_booking"] = *req.FriendlyBooking

	if err := database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update service settings",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Service settings updated successfully",
		"data": fiber.Map{
			"friendly_booking": *req.FriendlyBooking,
		},
	})
}
