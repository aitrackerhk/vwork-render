package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SoftDeleteConfig 軟刪除配置
type SoftDeleteConfig struct {
	TableName       string                                 // 資料表名稱
	TenantIDColumn  string                                 // 租戶 ID 欄位名稱，默認 "tenant_id"
	IDColumn        string                                 // 主鍵欄位名稱，默認 "id"
	BeforeTrash     func(c *fiber.Ctx, id uuid.UUID) error // 放入垃圾筒前的檢查
	BeforeRestore   func(c *fiber.Ctx, id uuid.UUID) error // 還原前的檢查
	BeforePermanent func(c *fiber.Ctx, id uuid.UUID) error // 永久刪除前的檢查
}

// IsSystemDefault 檢查記錄是否為系統預設（不可刪除）
func IsSystemDefault(db *gorm.DB, tableName string, id uuid.UUID, tenantID uuid.UUID) bool {
	var count int64
	db.Table(tableName).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Where("extra_fields->>'is_system_default' = 'true'").
		Count(&count)
	return count > 0
}

// TrashItem 將項目放入垃圾筒（設置 trashed_at 時間）
func TrashItem(config SoftDeleteConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := middleware.GetTenantID(c)
		if tenantID == uuid.Nil {
			return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
		}

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
		}

		// 設置默認值
		tenantCol := config.TenantIDColumn
		if tenantCol == "" {
			tenantCol = "tenant_id"
		}
		idCol := config.IDColumn
		if idCol == "" {
			idCol = "id"
		}

		// 檢查是否為系統預設資料
		if IsSystemDefault(database.DB, config.TableName, id, tenantID) {
			return c.Status(400).JSON(fiber.Map{"error": "Cannot delete system default data"})
		}

		// 執行自定義的前置檢查
		if config.BeforeTrash != nil {
			if err := config.BeforeTrash(c, id); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
		}

		// 設置 trashed_at 時間
		result := database.DB.Table(config.TableName).
			Where(fmt.Sprintf("%s = ? AND %s = ?", idCol, tenantCol), id, tenantID).
			Update("trashed_at", time.Now())

		if result.Error != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to trash item: %v", result.Error)})
		}

		if result.RowsAffected == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "Item not found"})
		}

		return c.JSON(fiber.Map{
			"message": "Item moved to trash",
			"info":    "Data will be automatically deleted after 7 days",
		})
	}
}

// RestoreItem 從垃圾筒還原項目（清除 trashed_at）
func RestoreItem(config SoftDeleteConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := middleware.GetTenantID(c)
		if tenantID == uuid.Nil {
			return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
		}

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
		}

		// 設置默認值
		tenantCol := config.TenantIDColumn
		if tenantCol == "" {
			tenantCol = "tenant_id"
		}
		idCol := config.IDColumn
		if idCol == "" {
			idCol = "id"
		}

		// 執行自定義的前置檢查
		if config.BeforeRestore != nil {
			if err := config.BeforeRestore(c, id); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
		}

		// 清除 trashed_at 時間
		result := database.DB.Table(config.TableName).
			Where(fmt.Sprintf("%s = ? AND %s = ? AND trashed_at IS NOT NULL", idCol, tenantCol), id, tenantID).
			Update("trashed_at", nil)

		if result.Error != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to restore item: %v", result.Error)})
		}

		if result.RowsAffected == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "Item not found in trash"})
		}

		return c.JSON(fiber.Map{"message": "Item restored successfully"})
	}
}

// PermanentDeleteItem 永久刪除項目（將 trashed_at 設為 7 天前，使其不再顯示在垃圾筒）
func PermanentDeleteItem(config SoftDeleteConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := middleware.GetTenantID(c)
		if tenantID == uuid.Nil {
			return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
		}

		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
		}

		// 設置默認值
		tenantCol := config.TenantIDColumn
		if tenantCol == "" {
			tenantCol = "tenant_id"
		}
		idCol := config.IDColumn
		if idCol == "" {
			idCol = "id"
		}

		// 執行自定義的前置檢查
		if config.BeforePermanent != nil {
			if err := config.BeforePermanent(c, id); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
		}

		// 將 trashed_at 設為 7 天前，使其不再顯示在垃圾筒
		// 這樣資料仍然存在但不會在垃圾筒中顯示
		sevenDaysAgo := time.Now().AddDate(0, 0, -8)
		result := database.DB.Table(config.TableName).
			Where(fmt.Sprintf("%s = ? AND %s = ? AND trashed_at IS NOT NULL", idCol, tenantCol), id, tenantID).
			Update("trashed_at", sevenDaysAgo)

		if result.Error != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete item: %v", result.Error)})
		}

		if result.RowsAffected == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "Item not found in trash"})
		}

		return c.JSON(fiber.Map{"message": "Item permanently deleted"})
	}
}

// GetTrashedItems 獲取垃圾筒中的項目（7 天內）
func GetTrashedItems(config SoftDeleteConfig, selectColumns string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := middleware.GetTenantID(c)
		if tenantID == uuid.Nil {
			return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
		}

		// 設置默認值
		tenantCol := config.TenantIDColumn
		if tenantCol == "" {
			tenantCol = "tenant_id"
		}

		if selectColumns == "" {
			selectColumns = "*"
		}

		// 分頁
		page := c.QueryInt("page", 1)
		limit := c.QueryInt("limit", 20)
		offset := (page - 1) * limit

		// 搜索
		search := c.Query("search")

		// 只顯示 7 天內的垃圾資料
		sevenDaysAgo := time.Now().AddDate(0, 0, -7)

		// 計算總數
		var total int64
		countQuery := database.DB.Table(config.TableName).
			Where(fmt.Sprintf("%s = ? AND trashed_at IS NOT NULL AND trashed_at > ?", tenantCol), tenantID, sevenDaysAgo)

		if search != "" {
			// 搜索通常針對 name, code 等欄位，具體由調用方處理
			countQuery = countQuery.Where("name ILIKE ?", "%"+search+"%")
		}
		countQuery.Count(&total)

		// 獲取資料
		var results []map[string]interface{}
		dataQuery := database.DB.Table(config.TableName).
			Select(selectColumns).
			Where(fmt.Sprintf("%s = ? AND trashed_at IS NOT NULL AND trashed_at > ?", tenantCol), tenantID, sevenDaysAgo)

		if search != "" {
			dataQuery = dataQuery.Where("name ILIKE ?", "%"+search+"%")
		}

		dataQuery.Order("trashed_at DESC").
			Offset(offset).
			Limit(limit).
			Find(&results)

		return c.JSON(fiber.Map{
			"data":    results,
			"total":   total,
			"page":    page,
			"limit":   limit,
			"trashed": true,
		})
	}
}

// ApplyNotTrashedCondition 為查詢添加排除垃圾資料的條件
func ApplyNotTrashedCondition(query *gorm.DB) *gorm.DB {
	return query.Where("trashed_at IS NULL")
}

// ApplyNotTrashedConditionWithAlias 為查詢添加排除垃圾資料的條件（帶表別名）
func ApplyNotTrashedConditionWithAlias(query *gorm.DB, tableAlias string) *gorm.DB {
	return query.Where(fmt.Sprintf("%s.trashed_at IS NULL", tableAlias))
}
