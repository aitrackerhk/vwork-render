package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetNotificationSettings 獲取系統提示設定
func GetNotificationSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var settings models.NotificationSettings
	err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error

	// 檢查租戶已啟用的模組，只返回相關的設定
	var enabledModules []string
	database.DB.Model(&models.TenantModule{}).
		Where("tenant_id = ? AND is_enabled = ?", tenantID, true).
		Pluck("module_code", &enabledModules)

	// 根據模組啟用狀態決定哪些設定應該顯示
	hasHR := false
	hasServiceOrders := false
	hasProjects := false

	for _, code := range enabledModules {
		if code == "hr" {
			hasHR = true
		}
		if code == "service_orders" {
			hasServiceOrders = true
		}
		if code == "projects" || code == "project" {
			hasProjects = true
		}
	}

	if err != nil {
		// 如果不存在，返回默認值（全部啟用），但仍需包含 modules 欄位
		return c.JSON(fiber.Map{
			"data": models.NotificationSettings{
				TenantID:                          tenantID,
				AttendanceNotificationsEnabled:   true,
				OrderNotificationsEnabled:        true,
				ServiceOrderNotificationsEnabled:  true,
				AppointmentNotificationsEnabled:   true,
				ProjectDueNotificationsEnabled:    true,
			},
			"modules": fiber.Map{
				"has_hr":            hasHR,
				"has_service_orders": hasServiceOrders,
				"has_projects":       hasProjects,
			},
		})
	}

	// 構建回應，包含模組狀態
	return c.JSON(fiber.Map{
		"data": settings,
		"modules": fiber.Map{
			"has_hr":            hasHR,
			"has_service_orders": hasServiceOrders,
			"has_projects":       hasProjects,
		},
	})
}

// UpdateNotificationSettings 更新系統提示設定
func UpdateNotificationSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		AttendanceNotificationsEnabled   *bool `json:"attendance_notifications_enabled"`
		OrderNotificationsEnabled        *bool `json:"order_notifications_enabled"`
		ServiceOrderNotificationsEnabled  *bool `json:"service_order_notifications_enabled"`
		AppointmentNotificationsEnabled   *bool `json:"appointment_notifications_enabled"`
		ProjectDueNotificationsEnabled    *bool `json:"project_due_notifications_enabled"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	var settings models.NotificationSettings
	err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error

	if err != nil {
		// 創建新設定
		settings = models.NotificationSettings{
			TenantID:                          tenantID,
			AttendanceNotificationsEnabled:   true,
			OrderNotificationsEnabled:        true,
			ServiceOrderNotificationsEnabled:  true,
			AppointmentNotificationsEnabled:   true,
			ProjectDueNotificationsEnabled:    true,
			CreatedAt:                         time.Now(),
			UpdatedAt:                         time.Now(),
		}
	}

	// 更新設定的值（只更新提供的欄位）
	if req.AttendanceNotificationsEnabled != nil {
		settings.AttendanceNotificationsEnabled = *req.AttendanceNotificationsEnabled
	}
	if req.OrderNotificationsEnabled != nil {
		settings.OrderNotificationsEnabled = *req.OrderNotificationsEnabled
	}
	if req.ServiceOrderNotificationsEnabled != nil {
		settings.ServiceOrderNotificationsEnabled = *req.ServiceOrderNotificationsEnabled
	}
	if req.AppointmentNotificationsEnabled != nil {
		settings.AppointmentNotificationsEnabled = *req.AppointmentNotificationsEnabled
	}
	if req.ProjectDueNotificationsEnabled != nil {
		settings.ProjectDueNotificationsEnabled = *req.ProjectDueNotificationsEnabled
	}

	settings.UpdatedAt = time.Now()

	if err := database.DB.Save(&settings).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update notification settings"})
	}

	return c.JSON(fiber.Map{"data": settings})
}

