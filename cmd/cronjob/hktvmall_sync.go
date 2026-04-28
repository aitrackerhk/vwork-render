package main

import (
	"log"
	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/services/hktvmall"
	"time"

	"github.com/google/uuid"
)

// syncHKTVmallOrdersForAllTenants 為所有啟用 HKTVmall 的租戶拉取新訂單
func syncHKTVmallOrdersForAllTenants() {
	log.Println("🏪 Running HKTVmall order sync job at", time.Now().Format("2006-01-02 15:04:05"))

	// 查詢所有啟用了 HKTVmall 訂單同步的租戶
	var tenants []models.Tenant
	if err := database.DB.
		Where("extra_fields->'product_sync'->'hktvmall'->>'enabled' = 'true'").
		Where("extra_fields->'product_sync'->'hktvmall'->>'app_id' IS NOT NULL").
		Where("extra_fields->'product_sync'->'hktvmall'->>'app_id' != ''").
		Find(&tenants).Error; err != nil {
		log.Printf("❌ Failed to query tenants for HKTVmall order sync: %v", err)
		return
	}

	if len(tenants) == 0 {
		return // 沒有啟用的租戶，靜默返回
	}

	for _, tenant := range tenants {
		syncHKTVmallOrdersForTenant(tenant)
	}
}

func syncHKTVmallOrdersForTenant(tenant models.Tenant) {
	settings := tenant.ExtraFields["product_sync"]
	if settings == nil {
		return
	}

	settingsMap, ok := settings.(map[string]interface{})
	if !ok {
		return
	}

	hktvSettings, ok := settingsMap["hktvmall"].(map[string]interface{})
	if !ok {
		return
	}

	client, err := getHKTVmallClient(hktvSettings)
	if err != nil {
		log.Printf("❌ Failed to create HKTVmall client for tenant %s: %v", tenant.ID, err)
		return
	}

	// 只拉取最近 1 天的訂單
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)
	ordersResp, err := client.GetOrderList(1, 50, "", &startTime, &endTime)
	if err != nil {
		log.Printf("❌ Failed to get HKTVmall orders for tenant %s: %v", tenant.ID, err)
		return
	}

	if len(ordersResp.Orders) > 0 {
		log.Printf("📦 Synced %d orders from HKTVmall for tenant %s", len(ordersResp.Orders), tenant.ID)
		for _, order := range ordersResp.Orders {
			syncHKTVmallOrderToVWork(tenant, order)
		}
	}
}

func getHKTVmallClient(settings map[string]interface{}) (*hktvmall.Client, error) {
	merchantID := parseStringHKTV(settings["merchant_id"], "")
	shopID := parseStringHKTV(settings["shop_id"], "")
	appID := parseStringHKTV(settings["app_id"], "")
	appSecret := parseStringHKTV(settings["app_secret"], "")
	apiBase := parseStringHKTV(settings["api_base"], "")

	return hktvmall.NewClient(merchantID, shopID, appID, appSecret, apiBase), nil
}

func parseStringHKTV(value interface{}, fallback string) string {
	if value == nil {
		return fallback
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fallback
}

// syncHKTVmallOrderToVWork 將 HKTVmall 訂單同步到 vWork
func syncHKTVmallOrderToVWork(tenant models.Tenant, order hktvmall.Order) {
	// 檢查訂單是否已存在（通過 extra_fields 中的 hktvmall_order_id 查詢）
	var existingOrder models.Order
	err := database.DB.
		Where("tenant_id = ?", tenant.ID).
		Where("extra_fields->>'hktvmall_order_id' = ?", order.OrderID).
		First(&existingOrder).Error

	if err == nil {
		// 訂單已存在，更新狀態
		if existingOrder.Status != order.Status {
			existingOrder.Status = order.Status
			if err := database.DB.Save(&existingOrder).Error; err != nil {
				log.Printf("⚠️ Failed to update HKTVmall order %s: %v", order.OrderID, err)
			} else {
				log.Printf("🔄 Updated HKTVmall order status: %s -> %s", order.OrderID, order.Status)
			}
		}
		return
	}

	// 創建新訂單
	newOrder := models.Order{
		TenantID:       tenant.ID,
		OrderNumber:    order.OrderNo,
		OrderDate:      time.Unix(order.CreateTime, 0),
		Status:         order.Status,
		TotalAmount:    order.TotalAmount,
		SourceType:     "hktvmall",
		ContactName:    order.ShippingInfo.RecipientName,
		ContactPhone:   order.ShippingInfo.Phone,
		ContactAddress: order.ShippingInfo.Address,
		ExtraFields: models.JSONB{
			"hktvmall_order_id": order.OrderID,
			"hktvmall_order":    order,
			"currency":          order.Currency,
			"shipping_info":     order.ShippingInfo,
			"payment_method":    order.PaymentMethod,
		},
	}

	if err := database.DB.Create(&newOrder).Error; err != nil {
		log.Printf("❌ Failed to create HKTVmall order %s: %v", order.OrderID, err)
		return
	}

	// 創建訂單項目
	for _, item := range order.Items {
		var productID *uuid.UUID

		// 嘗試查找對應的產品
		var product models.Product
		if err := database.DB.
			Where("tenant_id = ?", tenant.ID).
			Where("extra_fields->>'hktvmall_product_id' = ?", item.ProductID).
			First(&product).Error; err == nil {
			productID = &product.ID
		} else {
			// 如果找不到，嘗試用 SKU (ProductCode) 查找
			if err := database.DB.
				Where("tenant_id = ?", tenant.ID).
				Where("sku = ?", item.ProductCode).
				First(&product).Error; err == nil {
				productID = &product.ID
			}
		}

		orderItem := models.OrderItem{
			OrderID:    newOrder.ID,
			TenantID:   tenant.ID,
			ProductID:  productID,
			Quantity:   float64(item.Quantity),
			UnitPrice:  item.Price,
			TotalPrice: item.TotalPrice,
			ExtraFields: models.JSONB{
				"hktvmall_item_id":    item.ItemID,
				"product_code":        item.ProductCode,
				"product_name":        item.ProductName,
				"hktvmall_product_id": item.ProductID,
			},
		}
		if err := database.DB.Create(&orderItem).Error; err != nil {
			log.Printf("⚠️ Failed to create order item for order %s: %v", order.OrderID, err)
		}
	}

	log.Printf("✅ Created HKTVmall order: %s (%s)", order.OrderID, order.OrderNo)
}

// syncHKTVmallProductsForAllTenants 為所有啟用 HKTVmall 的租戶同步產品
func syncHKTVmallProductsForAllTenants() {
	log.Println("🏪 Running HKTVmall product sync job at", time.Now().Format("2006-01-02 15:04:05"))

	var tenants []models.Tenant
	if err := database.DB.
		Where("extra_fields->'product_sync'->'hktvmall'->>'enabled' = 'true'").
		Where("extra_fields->'product_sync'->'hktvmall'->>'sync_products' = 'true'").
		Where("extra_fields->'product_sync'->'hktvmall'->>'app_id' IS NOT NULL").
		Where("extra_fields->'product_sync'->'hktvmall'->>'app_id' != ''").
		Find(&tenants).Error; err != nil {
		log.Printf("❌ Failed to query tenants for HKTVmall product sync: %v", err)
		return
	}

	if len(tenants) == 0 {
		return
	}

	for _, tenant := range tenants {
		syncHKTVmallProductsForTenant(tenant)
	}
}

func syncHKTVmallProductsForTenant(tenant models.Tenant) {
	settings := tenant.ExtraFields["product_sync"]
	if settings == nil {
		return
	}

	settingsMap, ok := settings.(map[string]interface{})
	if !ok {
		return
	}

	hktvSettings, ok := settingsMap["hktvmall"].(map[string]interface{})
	if !ok {
		return
	}

	client, err := getHKTVmallClient(hktvSettings)
	if err != nil {
		log.Printf("❌ Failed to create HKTVmall client for tenant %s: %v", tenant.ID, err)
		return
	}

	// 從 HKTVmall 拉取產品列表
	page := 1
	pageSize := 50
	totalSynced := 0

	for {
		productsResp, err := client.GetProductList(page, pageSize, "active")
		if err != nil {
			log.Printf("❌ Failed to get HKTVmall products for tenant %s (page %d): %v", tenant.ID, page, err)
			break
		}

		for _, product := range productsResp.Products {
			syncHKTVmallProductToVWork(tenant, product)
			totalSynced++
		}

		if !productsResp.HasNextPage {
			break
		}
		page++
	}

	if totalSynced > 0 {
		log.Printf("📦 Synced %d products from HKTVmall for tenant %s", totalSynced, tenant.ID)
	}
}

// syncHKTVmallProductToVWork 將 HKTVmall 產品同步到 vWork
func syncHKTVmallProductToVWork(tenant models.Tenant, product hktvmall.Product) {
	// 檢查產品是否已存在（通過 hktvmall_product_id 查詢）
	var existingProduct models.Product
	err := database.DB.
		Where("tenant_id = ?", tenant.ID).
		Where("extra_fields->>'hktvmall_product_id' = ?", product.ProductID).
		First(&existingProduct).Error

	if err == nil {
		// 產品已存在，更新資料
		existingProduct.Name = product.Name
		existingProduct.Description = product.Description
		if err := database.DB.Save(&existingProduct).Error; err != nil {
			log.Printf("⚠️ Failed to update HKTVmall product %s: %v", product.ProductID, err)
		}
		return
	}

	// 創建新產品
	newProduct := models.Product{
		TenantID:    tenant.ID,
		SKU:         product.ProductCode,
		Name:        product.Name,
		Description: product.Description,
		Status:      product.Status,
		ExtraFields: models.JSONB{
			"hktvmall_product_id": product.ProductID,
			"hktvmall_category":   product.CategoryID,
			"hktvmall_brand":      product.Brand,
			"source":              "hktvmall",
		},
	}

	if err := database.DB.Create(&newProduct).Error; err != nil {
		log.Printf("❌ Failed to create HKTVmall product %s: %v", product.ProductID, err)
		return
	}

	log.Printf("✅ Created product from HKTVmall: %s (%s)", product.Name, product.ProductCode)
}
