package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/services/delivery"
	"nwork/internal/utils"
)

// ============================================
// 外賣訂單 API (新架構：整合到 orders 表)
// ============================================

// GetDeliveryOrdersV2 獲取外賣訂單列表（整合架構）
func GetDeliveryOrdersV2(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	if pageSize > 100 {
		pageSize = 100
	}

	// 查詢 orders 表，source_type = 'delivery'
	query := database.DB.
		Where("tenant_id = ? AND source_type = ?", tenantID, "delivery").
		Preload("DeliveryOrderDetail").
		Preload("OrderItems")

	// 平台過濾
	if platform := c.Query("platform"); platform != "" {
		query = query.Where("delivery_platform = ?", platform)
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 日期過濾
	if date := c.Query("date"); date != "" {
		query = query.Where("DATE(created_at) = ?", date)
	}

	// 搜尋
	if search := c.Query("search"); search != "" {
		query = query.Where("platform_order_id ILIKE ? OR contact_name ILIKE ? OR order_number ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// 總數
	var total int64
	query.Model(&models.Order{}).Count(&total)

	// 統計各狀態數量
	var stats []struct {
		Status string
		Count  int64
	}
	database.DB.Model(&models.Order{}).
		Select("status, count(*) as count").
		Where("tenant_id = ? AND source_type = ?", tenantID, "delivery").
		Group("status").
		Scan(&stats)

	statsMap := make(map[string]int64)
	for _, s := range stats {
		statsMap[s.Status] = s.Count
	}

	// 分頁查詢
	var orders []models.Order
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "查詢失敗"})
	}

	// 構建響應
	result := make([]fiber.Map, 0, len(orders))
	for _, order := range orders {
		item := fiber.Map{
			"id":                order.ID,
			"order_number":      order.OrderNumber,
			"platform":          order.DeliveryPlatform,
			"platform_order_id": order.PlatformOrderID,
			"status":            order.Status,
			"customer_name":     order.ContactName,
			"customer_phone":    order.ContactPhone,
			"total_amount":      order.TotalAmount,
			"order_time":        order.CreatedAt,
			"items":             formatOrderItems(order.OrderItems),
		}

		// 添加外賣補充信息
		if order.DeliveryOrderDetail != nil {
			item["delivery_type"] = order.DeliveryOrderDetail.DeliveryType
			item["rider_name"] = order.DeliveryOrderDetail.RiderName
			item["rider_phone"] = order.DeliveryOrderDetail.RiderPhone
			item["platform_fee"] = order.DeliveryOrderDetail.PlatformFee
			item["delivery_fee"] = order.DeliveryOrderDetail.DeliveryFee
		}

		result = append(result, item)
	}

	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)

	return c.JSON(fiber.Map{
		"data": result,
		"pagination": fiber.Map{
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"total_pages": totalPages,
		},
		"stats": statsMap,
	})
}

// GetDeliveryOrderV2 獲取單個外賣訂單
func GetDeliveryOrderV2(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var order models.Order
	err := database.DB.
		Where("id = ? AND tenant_id = ? AND source_type = ?", id, tenantID, "delivery").
		Preload("DeliveryOrderDetail").
		Preload("OrderItems").
		First(&order).Error

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "訂單不存在"})
	}

	// 獲取狀態歷史
	var history []models.DeliveryOrderStatusHistory
	database.DB.Where("order_id = ?", order.ID).Order("created_at ASC").Find(&history)

	return c.JSON(fiber.Map{
		"data":    formatDeliveryOrder(&order),
		"history": history,
	})
}

// AcceptDeliveryOrderV2 接受外賣訂單
func AcceptDeliveryOrderV2(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	var order models.Order
	err := database.DB.
		Where("id = ? AND tenant_id = ? AND source_type = ?", id, tenantID, "delivery").
		Preload("DeliveryOrderDetail").
		First(&order).Error

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "訂單不存在"})
	}

	if order.DeliveryPlatform == nil {
		return c.Status(400).JSON(fiber.Map{"error": "非外賣訂單"})
	}

	// 獲取整合設定
	var integration models.DeliveryIntegration
	if err := database.DB.Where("tenant_id = ? AND platform = ?", tenantID, *order.DeliveryPlatform).First(&integration).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "未找到平台整合設定"})
	}

	// 調用平台 API
	config := getIntegrationConfig(&integration)

	service := delivery.NewIntegrationService(tenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if order.PlatformOrderID != nil {
		if err := platformService.AcceptOrder(*order.PlatformOrderID); err != nil {
			log.Printf("[DeliveryOrder] 接受訂單失敗: %v", err)
			return c.Status(400).JSON(fiber.Map{"error": "接受訂單失敗: " + err.Error()})
		}
	}

	// 更新訂單狀態
	now := time.Now()
	order.Status = "accepted"
	order.UpdatedAt = now
	database.DB.Save(&order)

	// 更新外賣補充信息
	if order.DeliveryOrderDetail != nil {
		order.DeliveryOrderDetail.ConfirmedAt = &now
		database.DB.Save(order.DeliveryOrderDetail)
	}

	// 記錄狀態歷史
	database.DB.Create(&models.DeliveryOrderStatusHistory{
		OrderID:   order.ID,
		Status:    "accepted",
		Notes:     "訂單已接受",
		CreatedBy: &userID,
	})

	// 處理庫存扣減（基於產品映射）
	deductDeliveryOrderInventory(tenantID, order.ID, &integration)

	return c.JSON(fiber.Map{"success": true, "message": "訂單已接受"})
}

// RejectDeliveryOrderV2 拒絕外賣訂單
func RejectDeliveryOrderV2(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	var order models.Order
	err := database.DB.
		Where("id = ? AND tenant_id = ? AND source_type = ?", id, tenantID, "delivery").
		Preload("DeliveryOrderDetail").
		First(&order).Error

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "訂單不存在"})
	}

	if order.DeliveryPlatform == nil {
		return c.Status(400).JSON(fiber.Map{"error": "非外賣訂單"})
	}

	// 獲取整合設定
	var integration models.DeliveryIntegration
	if err := database.DB.Where("tenant_id = ? AND platform = ?", tenantID, *order.DeliveryPlatform).First(&integration).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "未找到平台整合設定"})
	}

	// 調用平台 API
	config := getIntegrationConfig(&integration)

	service := delivery.NewIntegrationService(tenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if order.PlatformOrderID != nil {
		if err := platformService.RejectOrder(*order.PlatformOrderID, req.Reason); err != nil {
			log.Printf("[DeliveryOrder] 拒絕訂單失敗: %v", err)
			return c.Status(400).JSON(fiber.Map{"error": "拒絕訂單失敗: " + err.Error()})
		}
	}

	// 更新訂單狀態
	now := time.Now()
	order.Status = "rejected"
	order.UpdatedAt = now
	database.DB.Save(&order)

	// 更新外賣補充信息
	if order.DeliveryOrderDetail != nil {
		order.DeliveryOrderDetail.CancelledAt = &now
		order.DeliveryOrderDetail.CancelReason = req.Reason
		order.DeliveryOrderDetail.CancelledBy = "merchant"
		database.DB.Save(order.DeliveryOrderDetail)
	}

	// 記錄狀態歷史
	database.DB.Create(&models.DeliveryOrderStatusHistory{
		OrderID:   order.ID,
		Status:    "rejected",
		Notes:     "訂單已拒絕: " + req.Reason,
		CreatedBy: &userID,
	})

	// 恢復已扣減的庫存
	go restoreDeliveryOrderInventory(tenantID, order.ID, &integration)

	return c.JSON(fiber.Map{"success": true, "message": "訂單已拒絕"})
}

// UpdateDeliveryOrderStatusV2 更新外賣訂單狀態
func UpdateDeliveryOrderStatusV2(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	var req struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	var order models.Order
	err := database.DB.
		Where("id = ? AND tenant_id = ? AND source_type = ?", id, tenantID, "delivery").
		Preload("DeliveryOrderDetail").
		First(&order).Error

	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "訂單不存在"})
	}

	if order.DeliveryPlatform == nil {
		return c.Status(400).JSON(fiber.Map{"error": "非外賣訂單"})
	}

	// 獲取整合設定
	var integration models.DeliveryIntegration
	if err := database.DB.Where("tenant_id = ? AND platform = ?", tenantID, *order.DeliveryPlatform).First(&integration).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "未找到平台整合設定"})
	}

	// 調用平台 API 更新狀態
	config := getIntegrationConfig(&integration)

	service := delivery.NewIntegrationService(tenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// 根據狀態調用對應的平台 API
	if order.PlatformOrderID != nil {
		switch req.Status {
		case "preparing":
			if err := platformService.MarkPreparing(*order.PlatformOrderID); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "更新狀態失敗: " + err.Error()})
			}
		case "ready", "ready_for_pickup":
			if err := platformService.MarkReadyForPickup(*order.PlatformOrderID); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "更新狀態失敗: " + err.Error()})
			}
		case "picked_up":
			// Rider picked up — local status update only, platform notifies via webhook
			if order.DeliveryOrderDetail != nil {
				now := time.Now()
				order.DeliveryOrderDetail.ActualPickupTime = &now
				database.DB.Save(order.DeliveryOrderDetail)
			}
		}
	}

	// 更新本地狀態
	order.Status = req.Status
	order.UpdatedAt = time.Now()
	database.DB.Save(&order)

	// 記錄狀態歷史
	database.DB.Create(&models.DeliveryOrderStatusHistory{
		OrderID:   order.ID,
		Status:    req.Status,
		Notes:     req.Notes,
		CreatedBy: &userID,
	})

	return c.JSON(fiber.Map{"success": true, "message": "狀態已更新"})
}

// SyncDeliveryOrdersV2 同步外賣訂單（整合架構）
func SyncDeliveryOrdersV2(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	platform := c.Query("platform")
	if platform == "" {
		return c.Status(400).JSON(fiber.Map{"error": "請指定平台"})
	}

	// 獲取整合設定
	var integration models.DeliveryIntegration
	if err := database.DB.Where("tenant_id = ? AND platform = ?", tenantID, platform).First(&integration).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "未找到平台整合設定"})
	}

	if !integration.IsEnabled || !integration.IsConnected {
		return c.Status(400).JSON(fiber.Map{"error": "整合未啟用或未連接"})
	}

	// 調用平台 API 獲取訂單
	config := getIntegrationConfig(&integration)

	service := delivery.NewIntegrationService(tenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// 獲取最近訂單
	since := time.Now().Add(-24 * time.Hour)
	if integration.LastSyncAt != nil {
		since = *integration.LastSyncAt
	}

	platformOrders, err := platformService.GetOrders(since, 100)
	if err != nil {
		log.Printf("[DeliverySync] 獲取訂單失敗: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": "獲取訂單失敗: " + err.Error()})
	}

	// 保存訂單到 orders 表
	syncedCount := 0
	for _, pOrder := range platformOrders {
		if err := createOrUpdateDeliveryOrder(tenantID, &integration, &pOrder); err != nil {
			log.Printf("[DeliverySync] 處理訂單失敗: %v", err)
			continue
		}
		syncedCount++
	}

	// 更新同步時間
	now := time.Now()
	integration.LastSyncAt = &now
	integration.LastError = ""
	database.DB.Save(&integration)

	return c.JSON(fiber.Map{
		"success":       true,
		"synced_count":  syncedCount,
		"total_fetched": len(platformOrders),
	})
}

// createOrUpdateDeliveryOrder 創建或更新外賣訂單（整合到 orders 表）
func createOrUpdateDeliveryOrder(tenantID uuid.UUID, integration *models.DeliveryIntegration, pOrder *delivery.Order) error {
	platform := string(pOrder.Platform)

	// 檢查是否已存在
	var existingOrder models.Order
	err := database.DB.Where("tenant_id = ? AND delivery_platform = ? AND platform_order_id = ?",
		tenantID, platform, pOrder.PlatformOrderID).First(&existingOrder).Error

	if err == nil {
		// 更新現有訂單狀態
		if existingOrder.Status != string(pOrder.Status) {
			existingOrder.Status = string(pOrder.Status)
			existingOrder.UpdatedAt = time.Now()
			return database.DB.Save(&existingOrder).Error
		}
		return nil
	}

	// 創建新訂單
	orderNumber := generateDeliveryOrderNumber(platform, pOrder.PlatformOrderID)

	order := models.Order{
		TenantID:         tenantID,
		OrderNumber:      orderNumber,
		OrderDate:        time.Now(),
		Status:           string(pOrder.Status),
		TotalAmount:      pOrder.TotalAmount,
		ContactName:      pOrder.CustomerName,
		ContactPhone:     pOrder.CustomerPhone,
		ContactAddress:   pOrder.CustomerAddress,
		Notes:            pOrder.CustomerNotes,
		SourceType:       "delivery",
		DeliveryPlatform: &platform,
		PlatformOrderID:  &pOrder.PlatformOrderID,
		StoreID:          integration.StoreID,
	}

	// 使用事務
	tx := database.DB.Begin()

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 創建訂單項目
	for _, item := range pOrder.Items {
		orderItem := models.OrderItem{
			TenantID:       tenantID,
			OrderID:        order.ID,
			Quantity:       float64(item.Quantity),
			UnitPrice:      item.UnitPrice,
			TotalPrice:     item.TotalPrice,
			Notes:          item.Notes,
			PlatformItemID: &item.PlatformItemID,
			ItemName:       &item.Name,
		}

		// 嘗試通過映射找到對應的 Product
		var mapping models.DeliveryProductMapping
		if err := tx.Where("tenant_id = ? AND platform = ? AND platform_item_id = ?",
			tenantID, platform, item.PlatformItemID).First(&mapping).Error; err == nil {
			orderItem.ProductID = mapping.ProductID
		}

		if err := tx.Create(&orderItem).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	// 創建外賣補充信息
	rawDataJSON, _ := json.Marshal(pOrder.RawData)
	var rawData models.JSONB
	json.Unmarshal(rawDataJSON, &rawData)

	detail := models.DeliveryOrderDetail{
		OrderID:             order.ID,
		IntegrationID:       &integration.ID,
		Platform:            models.DeliveryPlatform(platform),
		PlatformOrderID:     pOrder.PlatformOrderID,
		PlatformOrderNumber: pOrder.PlatformOrderNumber,
		PlatformStatus:      pOrder.PlatformStatus,
		DeliveryType:        pOrder.DeliveryType,
		RiderName:           pOrder.RiderName,
		RiderPhone:          pOrder.RiderPhone,
		RiderTrackingURL:    pOrder.RiderTrackingURL,
		PlatformFee:         pOrder.PlatformFee,
		DeliveryFee:         pOrder.DeliveryFee,
		PlatformDiscount:    pOrder.DiscountAmount,
		RawData:             rawData,
	}

	if err := tx.Create(&detail).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// DeliveryWebhookV2 處理外賣平台 Webhook（整合架構）
func DeliveryWebhookV2(c *fiber.Ctx) error {
	platform := c.Params("platform")

	// 讀取請求體
	body := c.Body()

	// 獲取簽名
	signature := c.Get("X-Signature")
	if signature == "" {
		signature = c.Get("X-Webhook-Signature")
	}

	log.Printf("[DeliveryWebhook] 收到 %s webhook", platform)

	// 解析 webhook 事件以獲取商戶信息
	var eventData map[string]interface{}
	if err := json.Unmarshal(body, &eventData); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的 JSON"})
	}

	// 從事件中找出商戶 ID
	merchantID := extractMerchantID(eventData)

	// 查找對應的整合
	var integration models.DeliveryIntegration
	query := database.DB.Where("platform = ?", platform)
	if merchantID != "" {
		query = query.Where("merchant_id = ?", merchantID)
	}
	if err := query.First(&integration).Error; err != nil {
		log.Printf("[DeliveryWebhook] 找不到對應的整合")
		return c.Status(404).JSON(fiber.Map{"error": "找不到對應的整合"})
	}

	// 驗證簽名
	config := getIntegrationConfig(&integration)

	service := delivery.NewIntegrationService(integration.TenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "服務初始化失敗"})
	}

	if integration.WebhookSecret != "" && !platformService.ValidateWebhook(signature, body) {
		log.Printf("[DeliveryWebhook] 簽名驗證失敗")
		return c.Status(401).JSON(fiber.Map{"error": "簽名驗證失敗"})
	}

	// 解析事件
	event, err := platformService.ParseWebhookEvent(body)
	if err != nil {
		log.Printf("[DeliveryWebhook] 解析事件失敗: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": "解析事件失敗"})
	}

	// 處理事件
	if err := processDeliveryWebhookEventV2(integration.TenantID, &integration, event); err != nil {
		log.Printf("[DeliveryWebhook] 處理事件失敗: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "處理事件失敗"})
	}

	return c.JSON(fiber.Map{"success": true})
}

// processDeliveryWebhookEventV2 處理 Webhook 事件（整合架構）
func processDeliveryWebhookEventV2(tenantID uuid.UUID, integration *models.DeliveryIntegration, event *delivery.WebhookEvent) error {
	log.Printf("[DeliveryWebhook] 處理事件: type=%s, order_id=%s, status=%s",
		event.EventType, event.OrderID, event.Status)

	platform := string(event.Platform)

	// 查找訂單
	var order models.Order
	err := database.DB.Where("tenant_id = ? AND delivery_platform = ? AND platform_order_id = ?",
		tenantID, platform, event.OrderID).Preload("DeliveryOrderDetail").First(&order).Error

	if err != nil {
		// 新訂單事件，創建訂單
		log.Printf("[DeliveryWebhook] 新訂單通知: %s", event.OrderID)

		// 從 event 創建訂單
		pOrder := &delivery.Order{
			Platform:        delivery.Platform(platform),
			PlatformOrderID: event.OrderID,
			Status:          event.Status,
			RawData:         event.ParsedData,
		}

		return createOrUpdateDeliveryOrder(tenantID, integration, pOrder)
	}

	// 更新訂單狀態
	oldStatus := order.Status
	order.Status = string(event.Status)
	order.UpdatedAt = time.Now()

	if err := database.DB.Save(&order).Error; err != nil {
		return err
	}

	// 更新補充信息
	if order.DeliveryOrderDetail != nil {
		order.DeliveryOrderDetail.PlatformStatus = event.EventType
		if event.Status == delivery.OrderStatusCancelled {
			now := time.Now()
			order.DeliveryOrderDetail.CancelledAt = &now
			order.DeliveryOrderDetail.CancelledBy = "platform"

			// Restore inventory for cancelled orders
			go restoreDeliveryOrderInventory(tenantID, order.ID, integration)
		}
		database.DB.Save(order.DeliveryOrderDetail)
	}

	// 記錄狀態變更
	if oldStatus != string(event.Status) {
		rawEventJSON, _ := json.Marshal(event.ParsedData)
		var rawEvent models.JSONB
		json.Unmarshal(rawEventJSON, &rawEvent)

		database.DB.Create(&models.DeliveryOrderStatusHistory{
			OrderID:        order.ID,
			Status:         string(event.Status),
			PlatformStatus: event.EventType,
			RawEvent:       rawEvent,
		})
	}

	return nil
}

// ============================================
// 產品映射 API
// ============================================

// GetDeliveryProductMappings 獲取產品映射列表
func GetDeliveryProductMappings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var mappings []models.DeliveryProductMapping
	query := database.DB.Where("tenant_id = ?", tenantID).Preload("Product")

	if platform := c.Query("platform"); platform != "" {
		query = query.Where("platform = ?", platform)
	}

	if err := query.Order("platform_item_name ASC").Find(&mappings).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "查詢失敗"})
	}

	return c.JSON(mappings)
}

// CreateDeliveryProductMapping 創建產品映射
func CreateDeliveryProductMapping(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Platform         string  `json:"platform"`
		PlatformItemID   string  `json:"platform_item_id"`
		PlatformItemName string  `json:"platform_item_name"`
		PlatformCategory string  `json:"platform_category"`
		ProductID        *string `json:"product_id"`
		PlatformPrice    float64 `json:"platform_price"`
		SyncStock        bool    `json:"sync_stock"`
		StockBuffer      int     `json:"stock_buffer"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	mapping := models.DeliveryProductMapping{
		TenantID:         tenantID,
		Platform:         models.DeliveryPlatform(req.Platform),
		PlatformItemID:   req.PlatformItemID,
		PlatformItemName: req.PlatformItemName,
		PlatformCategory: req.PlatformCategory,
		PlatformPrice:    req.PlatformPrice,
		SyncStock:        req.SyncStock,
		StockBuffer:      req.StockBuffer,
		IsActive:         true,
	}

	if req.ProductID != nil {
		productUUID, err := uuid.Parse(*req.ProductID)
		if err == nil {
			mapping.ProductID = &productUUID
		}
	}

	if err := database.DB.Create(&mapping).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return c.Status(409).JSON(fiber.Map{"error": "映射已存在"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "創建失敗"})
	}

	return c.Status(201).JSON(mapping)
}

// DeleteDeliveryProductMapping 刪除產品映射
func DeleteDeliveryProductMapping(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	result := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.DeliveryProductMapping{})
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "刪除失敗"})
	}
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "映射不存在"})
	}

	return c.JSON(fiber.Map{"success": true})
}

// ============================================
// 輔助函數
// ============================================

func formatOrderItems(items []models.OrderItem) []fiber.Map {
	result := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		name := ""
		if item.ItemName != nil {
			name = *item.ItemName
		} else if item.Product != nil {
			name = item.Product.Name
		}

		result = append(result, fiber.Map{
			"name":       name,
			"quantity":   item.Quantity,
			"unit_price": item.UnitPrice,
			"total":      item.TotalPrice,
			"options":    item.ItemOptions,
		})
	}
	return result
}

func formatDeliveryOrder(order *models.Order) fiber.Map {
	result := fiber.Map{
		"id":                order.ID,
		"order_number":      order.OrderNumber,
		"platform":          order.DeliveryPlatform,
		"platform_order_id": order.PlatformOrderID,
		"status":            order.Status,
		"customer_name":     order.ContactName,
		"customer_phone":    order.ContactPhone,
		"delivery_address":  order.ContactAddress,
		"notes":             order.Notes,
		"total_amount":      order.TotalAmount,
		"order_time":        order.CreatedAt,
		"items":             formatOrderItems(order.OrderItems),
	}

	if order.DeliveryOrderDetail != nil {
		d := order.DeliveryOrderDetail
		result["delivery_type"] = d.DeliveryType
		result["rider_name"] = d.RiderName
		result["rider_phone"] = d.RiderPhone
		result["rider_tracking_url"] = d.RiderTrackingURL
		result["platform_fee"] = d.PlatformFee
		result["delivery_fee"] = d.DeliveryFee
		result["platform_discount"] = d.PlatformDiscount
		result["estimated_pickup_time"] = d.EstimatedPickupTime
		result["estimated_delivery_time"] = d.EstimatedDeliveryTime
	}

	return result
}

func generateDeliveryOrderNumber(platform, platformOrderID string) string {
	prefix := "DL"
	switch platform {
	case "foodpanda":
		prefix = "FP"
	case "keeta":
		prefix = "KT"
	case "deliveroo":
		prefix = "DR"
	}
	// 取平台訂單號後6位
	suffix := platformOrderID
	if len(suffix) > 6 {
		suffix = suffix[len(suffix)-6:]
	}
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().Format("060102"), suffix)
}

func extractMerchantID(data map[string]interface{}) string {
	if mid, ok := data["merchant_id"].(string); ok {
		return mid
	}
	if mid, ok := data["vendor_id"].(string); ok {
		return mid
	}
	if mid, ok := data["site_id"].(string); ok {
		return mid
	}
	if mid, ok := data["restaurant_id"].(string); ok {
		return mid
	}
	return ""
}

func getIntegrationConfig(integration *models.DeliveryIntegration) delivery.IntegrationConfig {
	isSandbox := false
	if integration.Settings != nil {
		if env, ok := integration.Settings["environment"].(string); ok && env == "sandbox" {
			isSandbox = true
		}
	}

	return delivery.IntegrationConfig{
		Platform:      delivery.Platform(integration.Platform),
		MerchantID:    integration.MerchantID,
		APIKey:        integration.APIKey,
		APISecret:     integration.APISecret,
		AccessToken:   integration.AccessToken,
		RefreshToken:  integration.RefreshToken,
		WebhookSecret: integration.WebhookSecret,
		Sandbox:       isSandbox,
		ExtraSettings: integration.Settings,
	}
}

// ============================================
// 庫存同步邏輯
// ============================================

// deductDeliveryOrderInventory 根據產品映射扣減外賣訂單庫存
// 僅對設置 sync_inventory=true 的映射進行庫存扣減
func deductDeliveryOrderInventory(tenantID uuid.UUID, orderID uuid.UUID, integration *models.DeliveryIntegration) {
	// 獲取訂單項目
	var orderItems []models.OrderItem
	if err := database.DB.Where("order_id = ?", orderID).Find(&orderItems).Error; err != nil {
		log.Printf("[DeliveryInventory] 獲取訂單項目失敗: %v", err)
		return
	}

	// 獲取預設倉庫（從 store 關聯或使用租戶預設）
	warehouseID := getDefaultWarehouseForDelivery(tenantID, integration)
	if warehouseID == uuid.Nil {
		log.Printf("[DeliveryInventory] 找不到預設倉庫，跳過庫存扣減")
		return
	}

	for _, item := range orderItems {
		if item.PlatformItemID == nil || *item.PlatformItemID == "" {
			continue
		}

		// 查找產品映射
		var mapping models.DeliveryProductMapping
		platform := ""
		if integration != nil {
			platform = string(integration.Platform)
		}

		err := database.DB.Where(
			"tenant_id = ? AND platform = ? AND platform_item_id = ? AND sync_inventory = ?",
			tenantID, platform, *item.PlatformItemID, true,
		).First(&mapping).Error

		if err != nil {
			// 沒有映射或未啟用庫存同步，跳過
			continue
		}

		// 計算扣減數量（考慮數量比例）
		quantityToDeduct := int(item.Quantity)
		if mapping.QuantityRatio != nil && *mapping.QuantityRatio > 0 {
			quantityToDeduct = int(item.Quantity * (*mapping.QuantityRatio))
		}

		if quantityToDeduct <= 0 {
			continue
		}

		// 扣減庫存
		if err := utils.UpdateWarehouseStock(tenantID, *mapping.ProductID, warehouseID, quantityToDeduct, "decrease"); err != nil {
			log.Printf("[DeliveryInventory] 扣減庫存失敗 (product=%s, qty=%d): %v",
				mapping.ProductID, quantityToDeduct, err)
		} else {
			log.Printf("[DeliveryInventory] 扣減庫存成功 (product=%s, qty=%d, warehouse=%s)",
				mapping.ProductID, quantityToDeduct, warehouseID)
		}
	}
}

// restoreDeliveryOrderInventory 恢復已取消外賣訂單的庫存
func restoreDeliveryOrderInventory(tenantID uuid.UUID, orderID uuid.UUID, integration *models.DeliveryIntegration) {
	// 獲取訂單項目
	var orderItems []models.OrderItem
	if err := database.DB.Where("order_id = ?", orderID).Find(&orderItems).Error; err != nil {
		log.Printf("[DeliveryInventory] 獲取訂單項目失敗: %v", err)
		return
	}

	// 獲取預設倉庫
	warehouseID := getDefaultWarehouseForDelivery(tenantID, integration)
	if warehouseID == uuid.Nil {
		log.Printf("[DeliveryInventory] 找不到預設倉庫，跳過庫存恢復")
		return
	}

	for _, item := range orderItems {
		if item.PlatformItemID == nil || *item.PlatformItemID == "" {
			continue
		}

		// 查找產品映射
		var mapping models.DeliveryProductMapping
		platform := ""
		if integration != nil {
			platform = string(integration.Platform)
		}

		err := database.DB.Where(
			"tenant_id = ? AND platform = ? AND platform_item_id = ? AND sync_inventory = ?",
			tenantID, platform, *item.PlatformItemID, true,
		).First(&mapping).Error

		if err != nil {
			continue
		}

		// 計算恢復數量
		quantityToRestore := int(item.Quantity)
		if mapping.QuantityRatio != nil && *mapping.QuantityRatio > 0 {
			quantityToRestore = int(item.Quantity * (*mapping.QuantityRatio))
		}

		if quantityToRestore <= 0 {
			continue
		}

		// 恢復庫存
		if err := utils.UpdateWarehouseStock(tenantID, *mapping.ProductID, warehouseID, quantityToRestore, "increase"); err != nil {
			log.Printf("[DeliveryInventory] 恢復庫存失敗 (product=%s, qty=%d): %v",
				mapping.ProductID, quantityToRestore, err)
		} else {
			log.Printf("[DeliveryInventory] 恢復庫存成功 (product=%s, qty=%d)",
				mapping.ProductID, quantityToRestore)
		}
	}
}

// getDefaultWarehouseForDelivery 獲取外賣訂單的預設倉庫
func getDefaultWarehouseForDelivery(tenantID uuid.UUID, integration *models.DeliveryIntegration) uuid.UUID {
	// 優先使用租戶的預設倉庫
	var warehouse models.Warehouse
	if err := database.DB.Where("tenant_id = ? AND is_default = ?", tenantID, true).First(&warehouse).Error; err == nil {
		return warehouse.ID
	}

	// 使用第一個倉庫
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&warehouse).Error; err == nil {
		return warehouse.ID
	}

	return uuid.Nil
}

// SyncMenuHandler 同步菜單到外賣平台
func SyncMenuHandler(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	// Get integration
	var integration models.DeliveryIntegration
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&integration).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "整合不存在"})
	}

	if !integration.IsEnabled || !integration.IsConnected {
		return c.Status(400).JSON(fiber.Map{"error": "整合未啟用或未連接"})
	}

	// Read sync_menu_categories from integration settings
	var syncMenuCategories []string
	if integration.Settings != nil {
		if cats, ok := integration.Settings["sync_menu_categories"].([]interface{}); ok {
			for _, v := range cats {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					syncMenuCategories = append(syncMenuCategories, strings.TrimSpace(s))
				}
			}
		}
	}

	// Get all active product mappings for this integration/platform
	var mappings []models.DeliveryProductMapping
	query := database.DB.Where("tenant_id = ? AND platform = ? AND is_active = ?", tenantID, integration.Platform, true)
	if integration.ID != uuid.Nil {
		query = query.Where("integration_id = ?", integration.ID)
	}
	if err := query.Preload("Product").Find(&mappings).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "獲取產品映射失敗"})
	}

	// Convert to MenuProduct slice, filtering by sync_menu_categories if set
	var menuProducts []delivery.MenuProduct
	for _, m := range mappings {
		if m.Product == nil || m.ProductID == nil {
			continue
		}
		// Filter by category if sync_menu_categories is configured
		if len(syncMenuCategories) > 0 {
			productCat := strings.TrimSpace(m.Product.Category)
			matched := false
			for _, c := range syncMenuCategories {
				if strings.EqualFold(productCat, c) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		mp := delivery.MenuProduct{
			ID:          m.PlatformItemID,
			Name:        m.Product.Name,
			Description: m.Product.Description,
			Price:       m.PlatformPrice,
			ImageURL:    m.Product.ImageURL,
			Category:    m.PlatformCategory,
			Available:   m.IsActive,
		}
		if mp.Price == 0 {
			mp.Price = m.Product.Price
		}
		menuProducts = append(menuProducts, mp)
	}

	// Create platform service and sync
	config := getIntegrationConfig(&integration)

	service := delivery.NewIntegrationService(tenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := platformService.SyncMenu(menuProducts); err != nil {
		log.Printf("[DeliverySyncMenu] 同步菜單失敗 (platform=%s): %v", integration.Platform, err)
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "同步菜單失敗: " + err.Error(),
		})
	}

	// Update last sync time
	now := time.Now()
	integration.LastSyncAt = &now
	database.DB.Save(&integration)

	log.Printf("[DeliverySyncMenu] 同步菜單成功 (platform=%s, items=%d)", integration.Platform, len(menuProducts))
	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("已同步 %d 個菜單項目", len(menuProducts)),
	})
}

// UpdateItemAvailabilityHandler 更新單個商品的上下架狀態
func UpdateItemAvailabilityHandler(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var integration models.DeliveryIntegration
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&integration).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "整合不存在"})
	}

	var req struct {
		ItemID    string `json:"item_id"`
		Available bool   `json:"available"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}
	if req.ItemID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "item_id 為必填"})
	}

	config := getIntegrationConfig(&integration)

	service := delivery.NewIntegrationService(tenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := platformService.UpdateItemAvailability(req.ItemID, req.Available); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "更新商品狀態失敗: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "商品狀態已更新",
	})
}

// UpdateDeliveryProductMappingHandler 更新外賣產品映射
func UpdateDeliveryProductMappingHandler(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var mapping models.DeliveryProductMapping
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&mapping).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "產品映射不存在"})
	}

	var req struct {
		ProductID        *string  `json:"product_id"`
		PlatformItemID   string   `json:"platform_item_id"`
		PlatformItemName string   `json:"platform_item_name"`
		PlatformCategory string   `json:"platform_category"`
		PlatformPrice    *float64 `json:"platform_price"`
		SyncStock        *bool    `json:"sync_stock"`
		SyncInventory    *bool    `json:"sync_inventory"`
		StockBuffer      *int     `json:"stock_buffer"`
		QuantityRatio    *float64 `json:"quantity_ratio"`
		IsActive         *bool    `json:"is_active"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	// Update fields if provided
	if req.ProductID != nil {
		pid, err := uuid.Parse(*req.ProductID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "無效的 product_id"})
		}
		mapping.ProductID = &pid
	}
	if req.PlatformItemID != "" {
		mapping.PlatformItemID = req.PlatformItemID
	}
	if req.PlatformItemName != "" {
		mapping.PlatformItemName = req.PlatformItemName
	}
	if req.PlatformCategory != "" {
		mapping.PlatformCategory = req.PlatformCategory
	}
	if req.PlatformPrice != nil {
		mapping.PlatformPrice = *req.PlatformPrice
	}
	if req.SyncStock != nil {
		mapping.SyncStock = *req.SyncStock
	}
	if req.SyncInventory != nil {
		mapping.SyncInventory = *req.SyncInventory
	}
	if req.StockBuffer != nil {
		mapping.StockBuffer = *req.StockBuffer
	}
	if req.QuantityRatio != nil {
		mapping.QuantityRatio = req.QuantityRatio
	}
	if req.IsActive != nil {
		mapping.IsActive = *req.IsActive
	}

	if err := database.DB.Save(&mapping).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "更新失敗"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    mapping,
	})
}
