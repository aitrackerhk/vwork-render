package middleware

import (
	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ModuleCheckMiddleware 檢查模塊是否啟用的中間件
// 如果模塊未啟用，返回 403 錯誤
// 注意：為了向後兼容，如果租戶沒有配置模塊，默認認為啟用
func ModuleCheckMiddleware(moduleCode string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := GetTenantID(c)
		if tenantID == uuid.Nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Tenant not found",
			})
		}

		// 檢查模塊是否啟用
		var tenantModule models.TenantModule
		err := database.DB.Where("tenant_id = ? AND module_code = ? AND is_enabled = ?",
			tenantID, moduleCode, true).First(&tenantModule).Error

		if err != nil {
			// 如果沒有找到記錄，檢查是否有任何模塊配置
			var count int64
			database.DB.Model(&models.TenantModule{}).
				Where("tenant_id = ?", tenantID).
				Count(&count)

			// 如果租戶沒有任何模塊配置，默認啟用（向後兼容）
			if count == 0 {
				return c.Next()
			}

			// 如果有配置但該模塊未啟用，返回 403
			return c.Status(403).JSON(fiber.Map{
				"error":  "Module not enabled",
				"module": moduleCode,
			})
		}

		return c.Next()
	}
}

// IsModuleEnabled 輔助函數，檢查模塊是否啟用
// 返回 true 如果模塊啟用或租戶沒有模塊配置（向後兼容）
func IsModuleEnabled(c *fiber.Ctx, moduleCode string) bool {
	tenantID := GetTenantID(c)
	if tenantID == uuid.Nil {
		return false
	}

	// 檢查是否有模塊配置
	var count int64
	database.DB.Model(&models.TenantModule{}).
		Where("tenant_id = ?", tenantID).
		Count(&count)

	// 如果沒有配置，默認啟用（向後兼容）
	if count == 0 {
		return true
	}

	// 檢查該模塊是否啟用
	var tenantModule models.TenantModule
	err := database.DB.Where("tenant_id = ? AND module_code = ? AND is_enabled = ?",
		tenantID, moduleCode, true).First(&tenantModule).Error

	return err == nil
}

