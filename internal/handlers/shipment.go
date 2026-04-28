package handlers

import (
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/services/shipping"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// 配送 (Shipment) CRUD
// ============================================

// GetShipments 獲取配送列表
func GetShipments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var shipments []models.Shipment
	query := database.DB.Where("tenant_id = ?", tenantID)

	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where(
			"shipment_number ILIKE ? OR tracking_number ILIKE ? OR recipient_name ILIKE ? OR recipient_phone ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%",
		)
	}

	// 過濾狀態
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 過濾物流公司
	if logisticsCompanyID := c.Query("logistics_company_id"); logisticsCompanyID != "" {
		query = query.Where("logistics_company_id = ?", logisticsCompanyID)
	}

	// 過濾訂單
	if orderID := c.Query("order_id"); orderID != "" {
		query = query.Where("order_id = ?", orderID)
	}

	// 過濾出入貨記錄
	if inventoryMovementID := c.Query("inventory_movement_id"); inventoryMovementID != "" {
		query = query.Where("inventory_movement_id = ?", inventoryMovementID)
	}

	// 分頁
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Shipment{}).Count(&total)

	if err := query.
		Preload("LogisticsCompany").
		Preload("Order").
		Preload("CreatedByUser").
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&shipments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  shipments,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetShipment 獲取單個配送
func GetShipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var shipment models.Shipment
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("LogisticsCompany").
		Preload("Order").
		Preload("CreatedByUser").
		First(&shipment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Shipment not found"})
	}

	return c.JSON(shipment)
}

// CreateShipment 創建配送
func CreateShipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		LogisticsCompanyID  *uuid.UUID               `json:"logistics_company_id"`
		TrackingNumber      string                   `json:"tracking_number"`
		OrderID             *uuid.UUID               `json:"order_id"`
		InventoryMovementID *uuid.UUID               `json:"inventory_movement_id"`
		SenderName          string                   `json:"sender_name"`
		SenderPhone         string                   `json:"sender_phone"`
		SenderAddress       string                   `json:"sender_address"`
		RecipientName       string                   `json:"recipient_name"`
		RecipientPhone      string                   `json:"recipient_phone"`
		RecipientAddress    string                   `json:"recipient_address"`
		Weight              float64                  `json:"weight"`
		Dimensions          string                   `json:"dimensions"`
		ItemCount           int                      `json:"item_count"`
		Description         string                   `json:"description"`
		ShippingFee         float64                  `json:"shipping_fee"`
		InsuranceFee        float64                  `json:"insurance_fee"`
		Notes               string                   `json:"notes"`
		Items               []map[string]interface{} `json:"items"`
		ExtraFields         models.JSONB             `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 生成配送單號
	shipmentNumber := generateShipmentNumber(tenantID)

	// 計算總費用
	totalFee := req.ShippingFee + req.InsuranceFee

	// 處理空 UUID
	if req.OrderID != nil && *req.OrderID == uuid.Nil {
		req.OrderID = nil
	}
	if req.InventoryMovementID != nil && *req.InventoryMovementID == uuid.Nil {
		req.InventoryMovementID = nil
	}
	if req.LogisticsCompanyID != nil && *req.LogisticsCompanyID == uuid.Nil {
		req.LogisticsCompanyID = nil
	}

	// 檢查是否為外賣訂單 - 外賣訂單不能使用內置物流系統
	if req.OrderID != nil {
		var order models.Order
		if err := database.DB.Where("id = ? AND tenant_id = ?", *req.OrderID, tenantID).First(&order).Error; err == nil {
			if order.SourceType == "delivery" {
				return c.Status(400).JSON(fiber.Map{
					"error": "外賣訂單由外賣平台負責配送，不能使用內置物流系統",
				})
			}
		}
	}

	items := normalizeShipmentItems(req.Items, req.ExtraFields)
	if len(items) == 0 && req.OrderID != nil {
		items = buildShipmentItemsFromOrder(tenantID, *req.OrderID)
	}
	if len(items) > 0 {
		if req.ExtraFields == nil {
			req.ExtraFields = models.JSONB{}
		}
		req.ExtraFields["items"] = items
	}

	itemCount := req.ItemCount
	if itemCount < 1 {
		itemCount = sumShipmentItemCount(items)
		if itemCount < 1 {
			itemCount = 1
		}
	}

	shipment := models.Shipment{
		TenantID:            tenantID,
		ShipmentNumber:      shipmentNumber,
		LogisticsCompanyID:  req.LogisticsCompanyID,
		TrackingNumber:      req.TrackingNumber,
		OrderID:             req.OrderID,
		InventoryMovementID: req.InventoryMovementID,
		SenderName:          req.SenderName,
		SenderPhone:         req.SenderPhone,
		SenderAddress:       req.SenderAddress,
		RecipientName:       req.RecipientName,
		RecipientPhone:      req.RecipientPhone,
		RecipientAddress:    req.RecipientAddress,
		Weight:              req.Weight,
		Dimensions:          req.Dimensions,
		ItemCount:           itemCount,
		Description:         req.Description,
		ShippingFee:         req.ShippingFee,
		InsuranceFee:        req.InsuranceFee,
		TotalFee:            totalFee,
		Status:              "pending",
		Notes:               req.Notes,
		ExtraFields:         req.ExtraFields,
		CreatedBy:           &userID,
	}

	// 嘗試通過配送連接自動創建運單
	integrationResult := tryCreateShippingOrder(tenantID, req.LogisticsCompanyID, shipping.CreateOrderRequest{
		SenderName:       req.SenderName,
		SenderPhone:      req.SenderPhone,
		SenderAddress:    req.SenderAddress,
		RecipientName:    req.RecipientName,
		RecipientPhone:   req.RecipientPhone,
		RecipientAddress: req.RecipientAddress,
		Weight:           req.Weight,
		ItemCount:        itemCount,
		Description:      req.Description,
		Notes:            req.Notes,
	})

	if integrationResult != nil && integrationResult.Success {
		// 如果 API 創建成功，使用返回的物流單號
		shipment.TrackingNumber = integrationResult.TrackingNumber
		if integrationResult.EstimatedFee > 0 && shipment.ShippingFee == 0 {
			shipment.ShippingFee = integrationResult.EstimatedFee
			shipment.TotalFee = integrationResult.EstimatedFee + shipment.InsuranceFee
		}
		// 保存 API 響應信息到 extra_fields
		if shipment.ExtraFields == nil {
			shipment.ExtraFields = models.JSONB{}
		}
		shipment.ExtraFields["integration_order_ref"] = integrationResult.OrderRef
		if integrationResult.ShareLink != "" {
			shipment.ExtraFields["tracking_link"] = integrationResult.ShareLink
		}
		log.Printf("[Shipment] 自動創建運單成功: %s", integrationResult.TrackingNumber)
	}

	if err := database.DB.Create(&shipment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create shipment"})
	}

	// 創建狀態歷史
	historyDesc := "配送單已建立"
	if integrationResult != nil && integrationResult.Success {
		historyDesc = fmt.Sprintf("配送單已建立，運單號: %s", integrationResult.TrackingNumber)
	}
	history := models.ShipmentStatusHistory{
		ShipmentID:  shipment.ID,
		Status:      "pending",
		Description: historyDesc,
		OccurredAt:  time.Now(),
	}
	database.DB.Create(&history)

	// 重新載入關聯數據
	database.DB.Where("id = ?", shipment.ID).
		Preload("LogisticsCompany").
		Preload("Order").
		First(&shipment)

	// 返回結果，包含整合信息
	response := fiber.Map{
		"id":              shipment.ID,
		"shipment_number": shipment.ShipmentNumber,
		"tracking_number": shipment.TrackingNumber,
		"status":          shipment.Status,
		"shipment":        shipment,
	}
	if integrationResult != nil {
		response["integration"] = fiber.Map{
			"success": integrationResult.Success,
			"message": integrationResult.Message,
		}
	}

	return c.Status(201).JSON(response)
}

// UpdateShipment 更新配送
func UpdateShipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	var shipment models.Shipment
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&shipment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Shipment not found"})
	}

	var req struct {
		LogisticsCompanyID *uuid.UUID               `json:"logistics_company_id"`
		TrackingNumber     string                   `json:"tracking_number"`
		SenderName         string                   `json:"sender_name"`
		SenderPhone        string                   `json:"sender_phone"`
		SenderAddress      string                   `json:"sender_address"`
		RecipientName      string                   `json:"recipient_name"`
		RecipientPhone     string                   `json:"recipient_phone"`
		RecipientAddress   string                   `json:"recipient_address"`
		Weight             float64                  `json:"weight"`
		Dimensions         string                   `json:"dimensions"`
		ItemCount          int                      `json:"item_count"`
		Description        string                   `json:"description"`
		ShippingFee        float64                  `json:"shipping_fee"`
		InsuranceFee       float64                  `json:"insurance_fee"`
		Notes              string                   `json:"notes"`
		Items              []map[string]interface{} `json:"items"`
		ExtraFields        models.JSONB             `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 處理空 UUID
	if req.LogisticsCompanyID != nil && *req.LogisticsCompanyID == uuid.Nil {
		req.LogisticsCompanyID = nil
	}

	// 計算總費用
	totalFee := req.ShippingFee + req.InsuranceFee

	items := normalizeShipmentItems(req.Items, req.ExtraFields)
	if len(items) > 0 {
		if req.ExtraFields == nil {
			req.ExtraFields = models.JSONB{}
		}
		req.ExtraFields["items"] = items
	}

	itemCount := req.ItemCount
	if itemCount < 1 {
		itemCount = sumShipmentItemCount(items)
		if itemCount < 1 {
			itemCount = 1
		}
	}

	updates := map[string]interface{}{
		"logistics_company_id": req.LogisticsCompanyID,
		"tracking_number":      req.TrackingNumber,
		"sender_name":          req.SenderName,
		"sender_phone":         req.SenderPhone,
		"sender_address":       req.SenderAddress,
		"recipient_name":       req.RecipientName,
		"recipient_phone":      req.RecipientPhone,
		"recipient_address":    req.RecipientAddress,
		"weight":               req.Weight,
		"dimensions":           req.Dimensions,
		"item_count":           itemCount,
		"description":          req.Description,
		"shipping_fee":         req.ShippingFee,
		"insurance_fee":        req.InsuranceFee,
		"total_fee":            totalFee,
		"notes":                req.Notes,
		"extra_fields":         req.ExtraFields,
		"updated_by":           userID,
	}

	if err := database.DB.Model(&shipment).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update shipment"})
	}

	// 重新載入
	database.DB.Where("id = ?", shipment.ID).
		Preload("LogisticsCompany").
		Preload("Order").
		First(&shipment)

	return c.JSON(shipment)
}

// UpdateShipmentStatus 更新配送狀態
func UpdateShipmentStatus(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var shipment models.Shipment
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&shipment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Shipment not found"})
	}

	var req struct {
		Status      string `json:"status"`
		Location    string `json:"location"`
		Description string `json:"description"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	validStatuses := map[string]bool{
		"pending":          true,
		"picked_up":        true,
		"in_transit":       true,
		"out_for_delivery": true,
		"delivered":        true,
		"failed":           true,
		"returned":         true,
		"cancelled":        true,
	}

	if !validStatuses[req.Status] {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid status"})
	}

	oldStatus := shipment.Status
	now := time.Now()

	updates := map[string]interface{}{
		"status": req.Status,
	}

	// 根據狀態更新時間戳
	switch req.Status {
	case "picked_up":
		updates["picked_up_at"] = now
	case "delivered":
		updates["actual_delivery_at"] = now
	}

	if err := database.DB.Model(&shipment).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update status"})
	}

	// 創建狀態歷史
	description := req.Description
	if description == "" {
		description = fmt.Sprintf("狀態從 %s 變更為 %s", oldStatus, req.Status)
	}

	history := models.ShipmentStatusHistory{
		ShipmentID:  shipment.ID,
		Status:      req.Status,
		Location:    req.Location,
		Description: description,
		OccurredAt:  now,
	}
	database.DB.Create(&history)

	// 重新載入
	database.DB.Where("id = ?", shipment.ID).
		Preload("LogisticsCompany").
		Preload("Order").
		First(&shipment)

	return c.JSON(shipment)
}

// GetShipmentHistory 獲取配送狀態歷史
func GetShipmentHistory(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	// 驗證配送存在
	var shipment models.Shipment
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&shipment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Shipment not found"})
	}

	var history []models.ShipmentStatusHistory
	if err := database.DB.Where("shipment_id = ?", id).
		Order("occurred_at DESC").
		Find(&history).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(history)
}

// DeleteShipment 刪除配送
func DeleteShipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var shipment models.Shipment
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&shipment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Shipment not found"})
	}

	// 只允許刪除待處理或已取消的配送
	if shipment.Status != "pending" && shipment.Status != "cancelled" {
		return c.Status(400).JSON(fiber.Map{"error": "只能刪除待處理或已取消的配送"})
	}

	// 刪除狀態歷史
	database.DB.Where("shipment_id = ?", id).Delete(&models.ShipmentStatusHistory{})

	// 刪除配送
	if err := database.DB.Delete(&shipment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete shipment"})
	}

	return c.JSON(fiber.Map{"message": "Shipment deleted"})
}

// tryCreateShippingOrder 嘗試通過配送連接自動創建運單
// 返回 nil 表示沒有配置或不需要自動創建
func tryCreateShippingOrder(tenantID uuid.UUID, logisticsCompanyID *uuid.UUID, req shipping.CreateOrderRequest) *shipping.CreateOrderResult {
	if logisticsCompanyID == nil || *logisticsCompanyID == uuid.Nil {
		return nil
	}

	// 獲取物流公司信息
	var logisticsCompany models.LogisticsCompany
	if err := database.DB.Where("id = ?", *logisticsCompanyID).First(&logisticsCompany).Error; err != nil {
		log.Printf("[Shipment] 無法獲取物流公司: %v", err)
		return nil
	}

	// 檢查整合類型
	if logisticsCompany.IntegrationType == "" || logisticsCompany.IntegrationType == "none" {
		return nil
	}

	// 獲取配送連接配置
	var module models.TenantModule
	if err := database.DB.Where("tenant_id = ? AND module_code = ?", tenantID, ShippingIntegrationsModuleCode).
		First(&module).Error; err != nil {
		log.Printf("[Shipment] 無法獲取配送連接配置: %v", err)
		return nil
	}

	// 創建整合服務
	integrationService := shipping.NewShippingIntegrationService(tenantID, map[string]interface{}(module.Config))

	// 檢查是否啟用
	if !integrationService.IsIntegrationEnabled(logisticsCompany.IntegrationType) {
		log.Printf("[Shipment] 配送連接 %s 未啟用", logisticsCompany.IntegrationType)
		return nil
	}

	// 設置整合類型
	req.IntegrationType = logisticsCompany.IntegrationType

	// 創建運單
	result, err := integrationService.CreateOrder(req)
	if err != nil {
		log.Printf("[Shipment] 創建運單失敗: %v", err)
		return &shipping.CreateOrderResult{
			Success: false,
			Message: fmt.Sprintf("創建運單失敗: %v", err),
		}
	}

	return result
}

func normalizeShipmentItems(items []map[string]interface{}, extra models.JSONB) []map[string]interface{} {
	if len(items) > 0 {
		return items
	}
	if extra == nil {
		return nil
	}
	raw, ok := extra["items"]
	if !ok || raw == nil {
		return nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := []map[string]interface{}{}
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func buildShipmentItemsFromOrder(tenantID uuid.UUID, orderID uuid.UUID) []map[string]interface{} {
	if orderID == uuid.Nil {
		return nil
	}
	var order models.Order
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, orderID).
		Preload("OrderItems.Product").
		First(&order).Error; err != nil {
		return nil
	}
	items := []map[string]interface{}{}
	for _, it := range order.OrderItems {
		name := ""
		sku := ""
		if it.Product != nil {
			name = it.Product.Name
			sku = it.Product.SKU
		}
		item := map[string]interface{}{
			"product_id":   it.ProductID,
			"product_name": name,
			"sku":          sku,
			"quantity":     it.Quantity,
			"unit_price":   it.UnitPrice,
			"total_price":  it.TotalPrice,
		}
		if it.ExtraFields != nil {
			fields := map[string]interface{}(it.ExtraFields)
			if attrs, ok := fields["product_attributes"]; ok {
				item["product_attributes"] = attrs
			}
		}
		items = append(items, item)
	}
	return items
}

func sumShipmentItemCount(items []map[string]interface{}) int {
	if len(items) == 0 {
		return 0
	}
	sum := 0
	for _, it := range items {
		if it == nil {
			continue
		}
		if v, ok := it["quantity"]; ok {
			switch qty := v.(type) {
			case int:
				sum += qty
			case int64:
				sum += int(qty)
			case float64:
				sum += int(qty)
			case float32:
				sum += int(qty)
			case string:
				if n, err := strconv.Atoi(qty); err == nil {
					sum += n
				}
			}
		}
	}
	return sum
}

// CreateShipmentFromInventoryMovement 從出入貨記錄創建配送
// 由於 InventoryMovement 是動態計算的（不是資料庫模型），此 API 接收前端傳來的 movement 資訊
func CreateShipmentFromInventoryMovement(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	movementID := c.Params("id") // 前端傳來的 movement ID（用於檢查是否已創建配送）

	// 解析 UUID
	movementUUID, err := uuid.Parse(movementID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid movement ID"})
	}

	// 檢查是否已有配送
	var existingShipment models.Shipment
	if err := database.DB.Where("tenant_id = ? AND inventory_movement_id = ?", tenantID, movementID).
		First(&existingShipment).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{
			"error":    "此出入貨記錄已有配送單",
			"shipment": existingShipment,
		})
	}

	var req struct {
		// 來源資訊
		OrderID           *uuid.UUID  `json:"order_id"`
		ProductID         *uuid.UUID  `json:"product_id"`
		ProductName       string      `json:"product_name"`
		ProductAttributes interface{} `json:"product_attributes"`
		Quantity          int         `json:"quantity"`
		WarehouseID       *uuid.UUID  `json:"warehouse_id"`

		// 配送資訊
		LogisticsCompanyID *uuid.UUID `json:"logistics_company_id"`
		RecipientName      string     `json:"recipient_name"`
		RecipientPhone     string     `json:"recipient_phone"`
		RecipientAddress   string     `json:"recipient_address"`
		Notes              string     `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 處理空 UUID
	if req.LogisticsCompanyID != nil && *req.LogisticsCompanyID == uuid.Nil {
		req.LogisticsCompanyID = nil
	}
	if req.OrderID != nil && *req.OrderID == uuid.Nil {
		req.OrderID = nil
	}

	// 獲取租戶信息作為發件人
	var tenant models.Tenant
	database.DB.Where("id = ?", tenantID).First(&tenant)

	// 獲取倉庫作為發件地址
	senderAddress := ""
	if req.WarehouseID != nil {
		var warehouse models.Warehouse
		if err := database.DB.Where("id = ?", req.WarehouseID).First(&warehouse).Error; err == nil {
			senderAddress = warehouse.Address
		}
	}

	// 生成配送單號
	shipmentNumber := generateShipmentNumber(tenantID)

	// 構建產品明細
	items := []map[string]interface{}{}
	productName := req.ProductName
	productSKU := ""
	if req.ProductID != nil {
		var product models.Product
		if err := database.DB.Where("id = ?", req.ProductID).First(&product).Error; err == nil {
			if productName == "" {
				productName = product.Name
			}
			productSKU = product.SKU
		}
	}
	if productName != "" || req.ProductID != nil {
		items = append(items, map[string]interface{}{
			"product_id":         req.ProductID,
			"product_name":       productName,
			"sku":                productSKU,
			"quantity":           req.Quantity,
			"unit_price":         0,
			"total_price":        0,
			"product_attributes": req.ProductAttributes,
		})
	}

	// 構建描述
	itemCount := req.Quantity
	if itemCount < 1 {
		itemCount = 1
	}
	description := productName
	if description == "" {
		description = "出入貨轉配送"
	}
	if itemCount > 1 {
		description = fmt.Sprintf("%s x %d", description, itemCount)
	}

	extraFields := models.JSONB{}
	if len(items) > 0 {
		extraFields["items"] = items
	}

	shipment := models.Shipment{
		TenantID:            tenantID,
		ShipmentNumber:      shipmentNumber,
		LogisticsCompanyID:  req.LogisticsCompanyID,
		OrderID:             req.OrderID,
		InventoryMovementID: &movementUUID,
		SenderName:          tenant.Name,
		SenderAddress:       senderAddress,
		RecipientName:       req.RecipientName,
		RecipientPhone:      req.RecipientPhone,
		RecipientAddress:    req.RecipientAddress,
		ItemCount:           itemCount,
		Description:         description,
		Status:              "pending",
		Notes:               req.Notes,
		ExtraFields:         extraFields,
		CreatedBy:           &userID,
	}

	if err := database.DB.Create(&shipment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create shipment"})
	}

	// 創建狀態歷史
	history := models.ShipmentStatusHistory{
		ShipmentID:  shipment.ID,
		Status:      "pending",
		Description: "從出入貨記錄創建配送單",
		OccurredAt:  time.Now(),
	}
	database.DB.Create(&history)

	// 重新載入
	database.DB.Where("id = ?", shipment.ID).
		Preload("LogisticsCompany").
		First(&shipment)

	return c.Status(201).JSON(shipment)
}

// generateShipmentNumber 生成配送單號
func generateShipmentNumber(tenantID uuid.UUID) string {
	// 格式：SHP + 年月日 + 序號
	today := time.Now().Format("20060102")

	var count int64
	database.DB.Model(&models.Shipment{}).
		Where("tenant_id = ? AND DATE(created_at) = CURRENT_DATE", tenantID).
		Count(&count)

	return fmt.Sprintf("SHP%s%04d", today, count+1)
}
