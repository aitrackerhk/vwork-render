package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetShifts 獲取所有工作時段
func GetShifts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var shifts []models.Shift
	query := database.DB.Model(&models.Shift{}).Where("tenant_id = ?", tenantID)
	
	// 支持通過 is_default 參數過濾
	if isDefaultStr := c.Query("is_default"); isDefaultStr != "" {
		isDefault := isDefaultStr == "true" || isDefaultStr == "1"
		query = query.Where("is_default = ?", isDefault)
	}

	if err := query.Order("is_default DESC, name ASC").Find(&shifts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch shifts: " + err.Error()})
	}

	return c.JSON(fiber.Map{"data": shifts})
}

// GetShift 獲取單個工作時段
func GetShift(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid shift ID"})
	}

	var shift models.Shift
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&shift).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Shift not found"})
	}

	return c.JSON(shift)
}

// CreateShift 創建工作時段
func CreateShift(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		Name      string `json:"name"`
		StartTime string `json:"start_time"` // 格式：HH:MM，例如 "09:00"
		EndTime   string `json:"end_time"`   // 格式：HH:MM，例如 "18:00"
		IsDefault *bool  `json:"is_default"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 解析時間（支持 HH:MM 或 HH:MM:SS 格式）
	var startTime models.SQLTime
	var endTime models.SQLTime
	
	// 驗證並格式化開始時間
	if _, err := time.Parse("15:04", req.StartTime); err != nil {
		if _, err2 := time.Parse("15:04:05", req.StartTime); err2 != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid start_time format. Use HH:MM or HH:MM:SS"})
		}
		startTime = models.SQLTime(req.StartTime)
	} else {
		// 如果是 HH:MM 格式，轉換為 HH:MM:SS
		startTime = models.SQLTime(req.StartTime + ":00")
	}
	
	// 驗證並格式化結束時間
	if _, err := time.Parse("15:04", req.EndTime); err != nil {
		if _, err2 := time.Parse("15:04:05", req.EndTime); err2 != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid end_time format. Use HH:MM or HH:MM:SS"})
		}
		endTime = models.SQLTime(req.EndTime)
	} else {
		// 如果是 HH:MM 格式，轉換為 HH:MM:SS
		endTime = models.SQLTime(req.EndTime + ":00")
	}

	// 如果設為預設，先取消其他預設
	isDefault := req.IsDefault != nil && *req.IsDefault
	if isDefault {
		database.DB.Model(&models.Shift{}).Where("tenant_id = ?", tenantID).Update("is_default", false)
	}

	shift := models.Shift{
		TenantID:  tenantID,
		Name:      req.Name,
		StartTime: startTime,
		EndTime:   endTime,
		IsDefault: isDefault,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := database.DB.Create(&shift).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create shift"})
	}

	return c.Status(201).JSON(shift)
}

// UpdateShift 更新工作時段
func UpdateShift(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid shift ID"})
	}

	var shift models.Shift
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&shift).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Shift not found"})
	}

	var req struct {
		Name      *string `json:"name"`
		StartTime *string `json:"start_time"` // 格式：HH:MM
		EndTime   *string `json:"end_time"`   // 格式：HH:MM
		IsDefault *bool   `json:"is_default"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name != nil {
		shift.Name = *req.Name
	}

	if req.StartTime != nil {
		// 驗證並格式化開始時間（支持 HH:MM 或 HH:MM:SS 格式）
		if _, err := time.Parse("15:04", *req.StartTime); err != nil {
			if _, err2 := time.Parse("15:04:05", *req.StartTime); err2 != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid start_time format. Use HH:MM or HH:MM:SS"})
			}
			shift.StartTime = models.SQLTime(*req.StartTime)
		} else {
			// 如果是 HH:MM 格式，轉換為 HH:MM:SS
			shift.StartTime = models.SQLTime(*req.StartTime + ":00")
		}
	}

	if req.EndTime != nil {
		// 驗證並格式化結束時間（支持 HH:MM 或 HH:MM:SS 格式）
		if _, err := time.Parse("15:04", *req.EndTime); err != nil {
			if _, err2 := time.Parse("15:04:05", *req.EndTime); err2 != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid end_time format. Use HH:MM or HH:MM:SS"})
			}
			shift.EndTime = models.SQLTime(*req.EndTime)
		} else {
			// 如果是 HH:MM 格式，轉換為 HH:MM:SS
			shift.EndTime = models.SQLTime(*req.EndTime + ":00")
		}
	}

	if req.IsDefault != nil {
		isDefault := *req.IsDefault
		// 如果設為預設，先取消其他預設
		if isDefault {
			database.DB.Model(&models.Shift{}).Where("tenant_id = ? AND id != ?", tenantID, id).Update("is_default", false)
		}
		shift.IsDefault = isDefault
	}

	shift.UpdatedAt = time.Now()

	if err := database.DB.Save(&shift).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update shift"})
	}

	return c.JSON(shift)
}

// DeleteShift 刪除工作時段
func DeleteShift(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid shift ID"})
	}

	// 檢查是否有用戶使用此時段
	var count int64
	database.DB.Model(&models.User{}).Where("shift_id = ?", id).Count(&count)
	if count > 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot delete shift. There are users assigned to this shift"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.Shift{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete shift"})
	}

	return c.JSON(fiber.Map{"message": "Shift deleted successfully"})
}

