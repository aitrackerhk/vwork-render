package handlers

import (
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/services/amazon"
	"nwork/internal/services/lazada"
	"nwork/internal/services/rakuten"
	"nwork/internal/services/shopee"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetWarehouses 獲取倉庫列表
func GetWarehouses(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var warehouses []models.Warehouse
	query := database.DB.Where("tenant_id = ?", tenantID)

	// 搜索過濾
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Warehouse{}).Count(&total)

	// 按 is_default DESC 排序，確保默認值在第一個，然後按 created_at DESC 排序
	if err := query.Offset(offset).Limit(limit).Order("is_default DESC, created_at DESC").Find(&warehouses).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch warehouses: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  warehouses,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetWarehouse 獲取單個倉庫
func GetWarehouse(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid warehouse ID"})
	}

	var warehouse models.Warehouse
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&warehouse).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warehouse not found"})
	}

	return c.JSON(warehouse)
}

// CreateWarehouse 創建倉庫
func CreateWarehouse(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Name             string                 `json:"name"`
		Code             string                 `json:"code"`
		Address          string                 `json:"address"`
		ContactPerson    string                 `json:"contact_person"`
		PhoneCountryCode string                 `json:"phone_country_code"`
		Phone            string                 `json:"phone"`
		Email            string                 `json:"email"`
		Status           string                 `json:"status"`
		IsDefault        bool                   `json:"is_default"`
		ExtraFields      map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	// 自動生成倉庫編號（如果未提供）
	autoCode, err := generateAutoCode(tenantID, req.Code, autoCodeConfig{
		Prefix:     "WH-",
		FieldName:  "code",
		PageName:   "warehouses",
		Format:     codeFormatDate,
		TableModel: &models.Warehouse{},
		Column:     "code",
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate warehouse code: " + err.Error()})
	}

	// 檢查編號是否已存在
	var existing models.Warehouse
	if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, autoCode).First(&existing).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Warehouse code already exists"})
	}

	if req.Status == "" {
		req.Status = "active"
	}

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置
	if req.IsDefault {
		if err := tx.Model(&models.Warehouse{}).
			Where("tenant_id = ? AND is_default = ?", tenantID, true).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other warehouses: " + err.Error()})
		}
	}

	now := time.Now()
	warehouse := models.Warehouse{
		TenantID:         tenantID,
		Name:             req.Name,
		Code:             autoCode,
		Address:          req.Address,
		ContactPerson:    req.ContactPerson,
		PhoneCountryCode: req.PhoneCountryCode,
		Phone:            req.Phone,
		Email:            req.Email,
		Status:           req.Status,
		IsDefault:        req.IsDefault,
		CreatedBy:        &userID,
		UpdatedBy:        &userID,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExtraFields:      models.JSONB(req.ExtraFields),
	}

	if err := tx.Create(&warehouse).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create warehouse: " + err.Error()})
	}

	tx.Commit()

	// 成功建立後釋放預留編號
	releaseReservedCode(tenantID, "code", warehouse.Code)

	return c.Status(201).JSON(warehouse)
}

// UpdateWarehouse 更新倉庫
func UpdateWarehouse(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid warehouse ID"})
	}

	var warehouse models.Warehouse
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&warehouse).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warehouse not found"})
	}

	var req struct {
		Name             *string                 `json:"name"`
		Code             *string                 `json:"code"`
		Address          *string                 `json:"address"`
		ContactPerson    *string                 `json:"contact_person"`
		PhoneCountryCode *string                 `json:"phone_country_code"`
		Phone            *string                 `json:"phone"`
		Email            *string                 `json:"email"`
		Status           *string                 `json:"status"`
		IsDefault        *bool                   `json:"is_default"`
		ExtraFields      *map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.Name != nil {
		warehouse.Name = *req.Name
	}
	if req.Code != nil {
		// 檢查編號是否已被其他倉庫使用
		var existing models.Warehouse
		if err := database.DB.Where("tenant_id = ? AND code = ? AND id != ?", tenantID, *req.Code, id).First(&existing).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Warehouse code already exists"})
		}
		warehouse.Code = *req.Code
	}
	if req.Address != nil {
		warehouse.Address = *req.Address
	}
	if req.ContactPerson != nil {
		warehouse.ContactPerson = *req.ContactPerson
	}
	if req.PhoneCountryCode != nil {
		warehouse.PhoneCountryCode = *req.PhoneCountryCode
	}
	if req.Phone != nil {
		warehouse.Phone = *req.Phone
	}
	if req.Email != nil {
		warehouse.Email = *req.Email
	}
	if req.Status != nil {
		warehouse.Status = *req.Status
	}
	if req.IsDefault != nil {
		warehouse.IsDefault = *req.IsDefault
	}
	if req.ExtraFields != nil {
		warehouse.ExtraFields = models.JSONB(*req.ExtraFields)
	}

	warehouse.UpdatedBy = &userID
	warehouse.UpdatedAt = time.Now()

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置
	if req.IsDefault != nil && *req.IsDefault {
		if err := tx.Model(&models.Warehouse{}).
			Where("tenant_id = ? AND is_default = ? AND id != ?", tenantID, true, id).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other warehouses: " + err.Error()})
		}
	}

	if err := tx.Save(&warehouse).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update warehouse: " + err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	// 重新查詢以確保返回最新的數據（包括 is_default）
	var updatedWarehouse models.Warehouse
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&updatedWarehouse).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload warehouse"})
	}

	// 確保 is_default 字段被包含在響應中
	response := fiber.Map{
		"id":             updatedWarehouse.ID,
		"tenant_id":      updatedWarehouse.TenantID,
		"name":           updatedWarehouse.Name,
		"code":           updatedWarehouse.Code,
		"address":        updatedWarehouse.Address,
		"contact_person": updatedWarehouse.ContactPerson,
		"phone":          updatedWarehouse.Phone,
		"email":          updatedWarehouse.Email,
		"status":         updatedWarehouse.Status,
		"is_default":     updatedWarehouse.IsDefault,
		"created_at":     updatedWarehouse.CreatedAt,
		"updated_at":     updatedWarehouse.UpdatedAt,
		"created_by":     updatedWarehouse.CreatedBy,
		"updated_by":     updatedWarehouse.UpdatedBy,
	}
	if updatedWarehouse.ExtraFields != nil {
		response["extra_fields"] = updatedWarehouse.ExtraFields
	}

	return c.JSON(response)
}

// DeleteWarehouse 刪除倉庫
func DeleteWarehouse(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid warehouse ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.Warehouse{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete warehouse"})
	}

	return c.JSON(fiber.Map{"message": "Warehouse deleted successfully"})
}

// GetProductWarehouseStocks 獲取產品在各倉庫的庫存
func GetProductWarehouseStocks(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	productID := c.Query("product_id")
	warehouseID := c.Query("warehouse_id")

	var stocks []models.ProductWarehouseStock
	query := database.DB.Where("tenant_id = ?", tenantID).Preload("Product").Preload("Warehouse")

	if productID != "" {
		if id, err := uuid.Parse(productID); err == nil {
			query = query.Where("product_id = ?", id)
		}
	}

	if warehouseID != "" {
		if id, err := uuid.Parse(warehouseID); err == nil {
			query = query.Where("warehouse_id = ?", id)
		}
	}

	if err := query.Find(&stocks).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch stocks"})
	}

	return c.JSON(fiber.Map{"data": stocks})
}

// UpdateProductWarehouseStock 更新產品在指定倉庫的庫存
func UpdateProductWarehouseStock(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		ProductID        uuid.UUID `json:"product_id"`
		WarehouseID      uuid.UUID `json:"warehouse_id"`
		Quantity         int       `json:"quantity"`
		ReservedQuantity *int      `json:"reserved_quantity"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.ProductID == uuid.Nil || req.WarehouseID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Product ID and Warehouse ID are required"})
	}

	// 查找或創建庫存記錄
	var stock models.ProductWarehouseStock
	err := database.DB.Where("tenant_id = ? AND product_id = ? AND warehouse_id = ?", tenantID, req.ProductID, req.WarehouseID).First(&stock).Error

	if err != nil {
		// 創建新記錄
		stock = models.ProductWarehouseStock{
			TenantID:      tenantID,
			ProductID:     req.ProductID,
			WarehouseID:   req.WarehouseID,
			Quantity:      req.Quantity,
			LastUpdatedAt: time.Now(),
		}
		if req.ReservedQuantity != nil {
			stock.ReservedQuantity = *req.ReservedQuantity
		}
		if err := database.DB.Create(&stock).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create stock: " + err.Error()})
		}
	} else {
		// 更新現有記錄
		stock.Quantity = req.Quantity
		if req.ReservedQuantity != nil {
			stock.ReservedQuantity = *req.ReservedQuantity
		}
		stock.LastUpdatedAt = time.Now()
		if err := database.DB.Save(&stock).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update stock: " + err.Error()})
		}
	}

	// 重新載入關聯數據
	database.DB.Where("id = ?", stock.ID).Preload("Product").Preload("Warehouse").First(&stock)

	// 觸發 Shopee 庫存同步（非阻塞）
	go shopee.Sync.SyncInventoryToShopee(tenantID, req.ProductID, req.Quantity)
	// 觸發 Amazon 庫存同步（非阻塞）
	go amazon.Sync.SyncInventoryToAmazon(tenantID, req.ProductID, req.Quantity)
	// 觸發 Lazada 庫存同步（非阻塞）
	go lazada.Sync.SyncInventoryToLazada(tenantID, req.ProductID, req.Quantity)
	// 觸發 Rakuten 庫存同步（非阻塞）
	go func() {
		syncService, err := rakuten.NewSyncService(tenantID)
		if err != nil {
			log.Printf("[Rakuten] Failed to create sync service for tenant %s: %v", tenantID, err)
			return
		}
		if err := syncService.SyncInventory(req.ProductID, req.Quantity); err != nil {
			log.Printf("[Rakuten] Inventory sync failed for product %s: %v", req.ProductID, err)
		}
	}()

	return c.JSON(stock)
}

// ==================== 倉庫區 (WarehouseZone) Handlers ====================

// GetWarehouseZones 獲取倉庫區列表
func GetWarehouseZones(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var zones []models.WarehouseZone
	query := database.DB.Where("tenant_id = ? AND (trashed_at IS NULL)", tenantID).Preload("Warehouse")

	// 按倉庫過濾
	if warehouseID := c.Query("warehouse_id"); warehouseID != "" {
		if wid, err := uuid.Parse(warehouseID); err == nil {
			query = query.Where("warehouse_id = ?", wid)
		}
	}

	// 搜索過濾
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 100)
	offset := (page - 1) * limit

	var total int64
	countQuery := database.DB.Model(&models.WarehouseZone{}).Where("tenant_id = ? AND (trashed_at IS NULL)", tenantID)
	if warehouseID := c.Query("warehouse_id"); warehouseID != "" {
		if wid, err := uuid.Parse(warehouseID); err == nil {
			countQuery = countQuery.Where("warehouse_id = ?", wid)
		}
	}
	countQuery.Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("is_default DESC, created_at DESC").Find(&zones).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch warehouse zones: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  zones,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetWarehouseZone 獲取單個倉庫區
func GetWarehouseZone(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid warehouse zone ID"})
	}

	var zone models.WarehouseZone
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Preload("Warehouse").First(&zone).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warehouse zone not found"})
	}

	return c.JSON(zone)
}

// CreateWarehouseZone 創建倉庫區
func CreateWarehouseZone(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		WarehouseID string                 `json:"warehouse_id"`
		Name        string                 `json:"name"`
		Code        string                 `json:"code"`
		Description string                 `json:"description"`
		Status      string                 `json:"status"`
		IsDefault   bool                   `json:"is_default"`
		ExtraFields map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	warehouseID, err := uuid.Parse(req.WarehouseID)
	if err != nil || warehouseID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Warehouse ID is required"})
	}

	// 驗證倉庫存在
	var warehouse models.Warehouse
	if err := database.DB.Where("id = ? AND tenant_id = ?", warehouseID, tenantID).First(&warehouse).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Warehouse not found"})
	}

	if req.Status == "" {
		req.Status = "active"
	}

	tx := database.DB.Begin()

	// 如果設置為默認，取消該倉庫其他區的默認設置
	if req.IsDefault {
		if err := tx.Model(&models.WarehouseZone{}).
			Where("tenant_id = ? AND warehouse_id = ? AND is_default = ?", tenantID, warehouseID, true).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other zones: " + err.Error()})
		}
	}

	now := time.Now()
	zone := models.WarehouseZone{
		TenantID:    tenantID,
		WarehouseID: warehouseID,
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
		Status:      req.Status,
		IsDefault:   req.IsDefault,
		CreatedBy:   &userID,
		UpdatedBy:   &userID,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExtraFields: models.JSONB(req.ExtraFields),
	}

	if err := tx.Create(&zone).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create warehouse zone: " + err.Error()})
	}

	tx.Commit()

	// 重新載入關聯
	database.DB.Where("id = ?", zone.ID).Preload("Warehouse").First(&zone)

	return c.Status(201).JSON(zone)
}

// UpdateWarehouseZone 更新倉庫區
func UpdateWarehouseZone(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid warehouse zone ID"})
	}

	var zone models.WarehouseZone
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&zone).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Warehouse zone not found"})
	}

	var req struct {
		WarehouseID *string                 `json:"warehouse_id"`
		Name        *string                 `json:"name"`
		Code        *string                 `json:"code"`
		Description *string                 `json:"description"`
		Status      *string                 `json:"status"`
		IsDefault   *bool                   `json:"is_default"`
		ExtraFields *map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.WarehouseID != nil {
		if wid, err := uuid.Parse(*req.WarehouseID); err == nil {
			zone.WarehouseID = wid
		}
	}
	if req.Name != nil {
		zone.Name = *req.Name
	}
	if req.Code != nil {
		zone.Code = *req.Code
	}
	if req.Description != nil {
		zone.Description = *req.Description
	}
	if req.Status != nil {
		zone.Status = *req.Status
	}
	if req.IsDefault != nil {
		zone.IsDefault = *req.IsDefault
	}
	if req.ExtraFields != nil {
		zone.ExtraFields = models.JSONB(*req.ExtraFields)
	}

	zone.UpdatedBy = &userID
	zone.UpdatedAt = time.Now()

	tx := database.DB.Begin()

	// 如果設置為默認，取消該倉庫其他區的默認設置
	if req.IsDefault != nil && *req.IsDefault {
		if err := tx.Model(&models.WarehouseZone{}).
			Where("tenant_id = ? AND warehouse_id = ? AND is_default = ? AND id != ?", tenantID, zone.WarehouseID, true, id).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other zones: " + err.Error()})
		}
	}

	if err := tx.Save(&zone).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update warehouse zone: " + err.Error()})
	}

	tx.Commit()

	// 重新載入關聯
	database.DB.Where("id = ?", zone.ID).Preload("Warehouse").First(&zone)

	return c.JSON(zone)
}

// DeleteWarehouseZone 刪除倉庫區（軟刪除）
func DeleteWarehouseZone(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid warehouse zone ID"})
	}

	now := time.Now()
	if err := database.DB.Model(&models.WarehouseZone{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Update("trashed_at", now).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete warehouse zone"})
	}

	return c.JSON(fiber.Map{"message": "Warehouse zone deleted successfully"})
}

// ==================== 出入庫設定 (InventorySettings) Handlers ====================

// GetInventorySettings 獲取出入庫設定
func GetInventorySettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var settings models.InventorySettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error; err != nil {
		// 如果沒有設定，返回默認值
		settings = models.InventorySettings{
			TenantID:             tenantID,
			RequiresOutbound:     true,
			RequiresInbound:      true,
			AutoCompleteIfNoNeed: true,
		}
	}

	return c.JSON(settings)
}

// UpdateInventorySettings 更新出入庫設定
func UpdateInventorySettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		RequiresOutbound           *bool                   `json:"requires_outbound"`
		RequiresInbound            *bool                   `json:"requires_inbound"`
		RequiresItemByItemOutbound *bool                   `json:"requires_item_by_item_outbound"`
		RequiresItemByItemInbound  *bool                   `json:"requires_item_by_item_inbound"`
		AutoCompleteIfNoNeed       *bool                   `json:"auto_complete_if_no_need"`
		OutboundPrefix             *string                 `json:"outbound_prefix"`
		InboundPrefix              *string                 `json:"inbound_prefix"`
		ExtraFields                *map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	var settings models.InventorySettings
	err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error

	if err != nil {
		// 創建新設定
		settings = models.InventorySettings{
			TenantID:             tenantID,
			RequiresOutbound:     true,
			RequiresInbound:      true,
			AutoCompleteIfNoNeed: true,
			CreatedBy:            &userID,
			UpdatedBy:            &userID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
	}

	if req.RequiresOutbound != nil {
		settings.RequiresOutbound = *req.RequiresOutbound
	}
	if req.RequiresInbound != nil {
		settings.RequiresInbound = *req.RequiresInbound
	}
	if req.RequiresItemByItemOutbound != nil {
		settings.RequiresItemByItemOutbound = *req.RequiresItemByItemOutbound
	}
	if req.RequiresItemByItemInbound != nil {
		settings.RequiresItemByItemInbound = *req.RequiresItemByItemInbound
	}
	if req.AutoCompleteIfNoNeed != nil {
		settings.AutoCompleteIfNoNeed = *req.AutoCompleteIfNoNeed
	}
	if req.OutboundPrefix != nil {
		// 儲存到 ExtraFields
		if settings.ExtraFields == nil {
			settings.ExtraFields = models.JSONB{}
		}
		settings.ExtraFields["outbound_prefix"] = *req.OutboundPrefix
	}
	if req.InboundPrefix != nil {
		if settings.ExtraFields == nil {
			settings.ExtraFields = models.JSONB{}
		}
		settings.ExtraFields["inbound_prefix"] = *req.InboundPrefix
	}
	if req.ExtraFields != nil {
		settings.ExtraFields = models.JSONB(*req.ExtraFields)
	}

	settings.UpdatedBy = &userID
	settings.UpdatedAt = time.Now()

	if err != nil {
		// 創建
		if err := database.DB.Create(&settings).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create inventory settings: " + err.Error()})
		}
	} else {
		// 更新
		if err := database.DB.Save(&settings).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update inventory settings: " + err.Error()})
		}
	}

	return c.JSON(settings)
}
