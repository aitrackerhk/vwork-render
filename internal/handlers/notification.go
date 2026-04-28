package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetNotificationAlerts 獲取提示資訊列表
func GetNotificationAlerts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	query := database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID)

	// 過濾已讀/未讀
	if isRead := c.Query("is_read"); isRead != "" {
		if isRead == "true" {
			query = query.Where("is_read = ?", true)
		} else {
			query = query.Where("is_read = ?", false)
		}
	}

	// 過濾類型
	if alertType := c.Query("type"); alertType != "" {
		query = query.Where("type = ?", alertType)
	}

	// 分頁
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.NotificationAlert{}).Count(&total)

	var alerts []models.NotificationAlert
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&alerts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  alerts,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetNotificationAlert 獲取單個提示資訊
func GetNotificationAlert(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	alertID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid alert ID"})
	}

	var alert models.NotificationAlert
	if err := database.DB.Where("id = ? AND tenant_id = ? AND user_id = ?", alertID, tenantID, userID).
		First(&alert).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Alert not found"})
	}

	return c.JSON(alert)
}

// MarkNotificationAlertAsRead 標記提示資訊為已讀
func MarkNotificationAlertAsRead(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	alertID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid alert ID"})
	}

	now := time.Now()
	result := database.DB.Model(&models.NotificationAlert{}).
		Where("id = ? AND tenant_id = ? AND user_id = ?", alertID, tenantID, userID).
		Updates(map[string]interface{}{
			"is_read":  true,
			"read_at":  now,
			"updated_at": now,
		})

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": result.Error.Error()})
	}

	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Alert not found"})
	}

	return c.JSON(fiber.Map{"success": true})
}

// MarkAllNotificationAlertsAsRead 標記所有提示資訊為已讀
func MarkAllNotificationAlertsAsRead(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	now := time.Now()
	result := database.DB.Model(&models.NotificationAlert{}).
		Where("tenant_id = ? AND user_id = ? AND is_read = ?", tenantID, userID, false).
		Updates(map[string]interface{}{
			"is_read":  true,
			"read_at":  now,
			"updated_at": now,
		})

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": result.Error.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "count": result.RowsAffected})
}

// GetUnreadNotificationCount 獲取未讀提示資訊數
func GetUnreadNotificationCount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.JSON(fiber.Map{"count": 0})
	}

	var count int64
	if err := database.DB.Model(&models.NotificationAlert{}).
		Where("tenant_id = ? AND user_id = ? AND is_read = ?", tenantID, userID, false).
		Count(&count).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"count": count})
}

