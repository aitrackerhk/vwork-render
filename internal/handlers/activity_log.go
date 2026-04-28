package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetActivityLogs 獲取活動記錄列表
func GetActivityLogs(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var logs []models.ActivityLog
	query := database.DB.Where("tenant_id = ?", tenantID).
		Preload("User").
		Order("created_at DESC")

	// 根據用戶過濾
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if userID, err := uuid.Parse(userIDStr); err == nil {
			query = query.Where("user_id = ?", userID)
		}
	}

	// 根據操作類型過濾
	if action := c.Query("action"); action != "" {
		query = query.Where("action = ?", action)
	}

	// 根據資源類型過濾
	if resourceType := c.Query("resource_type"); resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}

	// 根據資源 ID 過濾
	if resourceIDStr := c.Query("resource_id"); resourceIDStr != "" {
		if resourceID, err := uuid.Parse(resourceIDStr); err == nil {
			query = query.Where("resource_id = ?", resourceID)
		}
	}

	// 時間範圍過濾
	if startDate := c.Query("start_date"); startDate != "" {
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if endDate := c.Query("end_date"); endDate != "" {
		if t, err := time.Parse("2006-01-02", endDate); err == nil {
			// 加上 23:59:59
			t = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			query = query.Where("created_at <= ?", t)
		}
	}

	// 搜索（在描述中）
	if search := c.Query("search"); search != "" {
		query = query.Where("description ILIKE ?", "%"+search+"%")
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ActivityLog{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch activity logs"})
	}

	return c.JSON(fiber.Map{
		"data":  logs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetActivityLog 獲取單個活動記錄
func GetActivityLog(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid activity log ID"})
	}

	var log models.ActivityLog
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("User").
		First(&log).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Activity log not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch activity log"})
	}

	return c.JSON(log)
}

// GetActivityLogStats 獲取活動記錄統計
func GetActivityLogStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var stats struct {
		TotalLogs    int64 `json:"total_logs"`
		TotalUsers   int64 `json:"total_users"`
		TotalActions int64 `json:"total_actions"`
		RecentLogs   int64 `json:"recent_logs"` // 最近 24 小時
	}

	// 總記錄數
	database.DB.Model(&models.ActivityLog{}).
		Where("tenant_id = ?", tenantID).
		Count(&stats.TotalLogs)

	// 總用戶數（有活動記錄的）
	database.DB.Model(&models.ActivityLog{}).
		Where("tenant_id = ?", tenantID).
		Distinct("user_id").
		Count(&stats.TotalUsers)

	// 總操作數（今天）
	today := time.Now().Truncate(24 * time.Hour)
	database.DB.Model(&models.ActivityLog{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, today).
		Count(&stats.TotalActions)

	// 最近 24 小時
	last24Hours := time.Now().Add(-24 * time.Hour)
	database.DB.Model(&models.ActivityLog{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, last24Hours).
		Count(&stats.RecentLogs)

	return c.JSON(stats)
}

// ActivityLogsPage 活動記錄頁面
func ActivityLogsPage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		// Preserve query parameters (e.g. ?product=vai) when redirecting to login
		loginURL := "/login"
		if rawQuery := string(c.Request().URI().QueryString()); rawQuery != "" {
			loginURL += "?" + rawQuery
		}
		return c.Redirect(loginURL)
	}

	return c.Render("pages/activity_logs", fiber.Map{
		"Title": "活動記錄",
	}, "layouts/cms_layout")
}
