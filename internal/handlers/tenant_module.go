package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetTenantModules 獲取當前租戶的所有模塊配置
func GetTenantModules(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var modules []models.TenantModule
	query := database.DB.Where("tenant_id = ?", tenantID)

	// 過濾啟用狀態
	isEnabled := c.Query("is_enabled")
	if isEnabled != "" {
		enabled, _ := strconv.ParseBool(isEnabled)
		query = query.Where("is_enabled = ?", enabled)
	}

	if err := query.Order("module_code ASC").Find(&modules).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch tenant modules",
		})
	}

	return c.JSON(fiber.Map{
		"data": modules,
	})
}

// GetTenantModule 獲取單個模塊配置
func GetTenantModule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	moduleCode := c.Params("moduleCode")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var module models.TenantModule
	if err := database.DB.Where("tenant_id = ? AND module_code = ?", tenantID, moduleCode).
		First(&module).Error; err != nil {
		// 如果不存在，返回默認啟用狀態（向後兼容）
		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"module_code": moduleCode,
				"is_enabled":  true, // 默認啟用，保持向後兼容
			},
		})
	}

	return c.JSON(fiber.Map{
		"data": module,
	})
}

// UpdateTenantModule 更新或創建模塊配置
func UpdateTenantModule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	moduleCode := c.Params("moduleCode")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var req struct {
		IsEnabled bool                   `json:"is_enabled"`
		Config    map[string]interface{} `json:"config"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// 查找或創建模塊
	var module models.TenantModule
	if err := database.DB.Where("tenant_id = ? AND module_code = ?", tenantID, moduleCode).
		First(&module).Error; err != nil {
		// 不存在，創建新的
		module = models.TenantModule{
			TenantID:   tenantID,
			ModuleCode: moduleCode,
			IsEnabled:  req.IsEnabled,
			Config:     models.JSONB(req.Config),
		}
		if err := database.DB.Create(&module).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to create module",
			})
		}
	} else {
		// 更新現有的
		updates := map[string]interface{}{
			"is_enabled": req.IsEnabled,
		}
		if req.Config != nil {
			updates["config"] = models.JSONB(req.Config)
		}

		if err := database.DB.Model(&module).Updates(updates).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to update module",
			})
		}
	}

	return c.JSON(fiber.Map{
		"message": "Module updated successfully",
		"data":    module,
	})
}

// BatchUpdateTenantModules 批量更新模塊配置
func BatchUpdateTenantModules(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var req struct {
		Modules []struct {
			ModuleCode string                 `json:"module_code"`
			IsEnabled  bool                   `json:"is_enabled"`
			Config     map[string]interface{} `json:"config"`
		} `json:"modules"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, moduleReq := range req.Modules {
		var module models.TenantModule
		if err := tx.Where("tenant_id = ? AND module_code = ?", tenantID, moduleReq.ModuleCode).
			First(&module).Error; err != nil {
			// 創建新的
			module = models.TenantModule{
				TenantID:   tenantID,
				ModuleCode: moduleReq.ModuleCode,
				IsEnabled:  moduleReq.IsEnabled,
				Config:     models.JSONB(moduleReq.Config),
			}
			if err := tx.Create(&module).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to create module: " + moduleReq.ModuleCode,
				})
			}
		} else {
			// 更新現有的
			updates := map[string]interface{}{
				"is_enabled": moduleReq.IsEnabled,
			}
			if moduleReq.Config != nil {
				updates["config"] = models.JSONB(moduleReq.Config)
			}
			if err := tx.Model(&module).Updates(updates).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to update module: " + moduleReq.ModuleCode,
				})
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update modules",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Modules updated successfully",
	})
}

// IsModuleEnabled 檢查模塊是否啟用（輔助函數，也可作為 API）
func IsModuleEnabled(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	moduleCode := c.Params("moduleCode")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var count int64
	database.DB.Model(&models.TenantModule{}).
		Where("tenant_id = ? AND module_code = ? AND is_enabled = ?", tenantID, moduleCode, true).
		Count(&count)

	return c.JSON(fiber.Map{
		"is_enabled": count > 0,
	})
}

