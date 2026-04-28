package main

import (
	"github.com/google/uuid"
	"log"
	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/services/shopee"
	"strconv"
	"time"
)

// syncShopeeOrdersForAllTenants 為所有啟用 Shopee 的租戶拉取新訂單
func syncShopeeOrdersForAllTenants() {
	log.Println("🛒 Running Shopee order sync job at", time.Now().Format("2006-01-02 15:04:05"))

	// 查詢所有啟用了 Shopee 訂單同步的租戶
	var tenants []models.Tenant
	if err := database.DB.
		Where("extra_fields->'product_sync'->'shopee'->>'enabled' = 'true'").
		Where("extra_fields->'product_sync'->'shopee'->>'sync_orders' = 'true'").
		Where("extra_fields->'product_sync'->'shopee'->>'access_token' IS NOT NULL").
		Where("extra_fields->'product_sync'->'shopee'->>'access_token' != ''").
		Find(&tenants).Error; err != nil {
		log.Printf("❌ Failed to query tenants for Shopee order sync: %v", err)
		return
	}

	if len(tenants) == 0 {
		return // 沒有啟用的租戶，靜默返回
	}

	for _, tenant := range tenants {
		syncShopeeOrdersForTenant(tenant)
	}
}

func syncShopeeOrdersForTenant(tenant models.Tenant) {
	settings := tenant.ExtraFields["product_sync"]
	if settings == nil {
		return
	}

	settingsMap, ok := settings.(map[string]interface{})
	if !ok {
		return
	}

	shopeeSettings, ok := settingsMap["shopee"].(map[string]interface{})
	if !ok {
		return
	}

	client, err := getShopeeClient(shopeeSettings)
	if err != nil {
		log.Printf("❌ Failed to create Shopee client for tenant %s: %v", tenant.ID, err)
		return
	}

	// 只拉取最近 1 天的訂單（因為每 15 分鐘執行一次）
	orders, err := client.GetRecentOrders(1, "")
	if err != nil {
		log.Printf("❌ Failed to get Shopee orders for tenant %s: %v", tenant.ID, err)
		return
	}

	if len(orders) > 0 {
		log.Printf("📦 Synced %d orders from Shopee for tenant %s", len(orders), tenant.ID)
		saveShopeeOrders(tenant, orders)
	}
}

func saveShopeeOrders(tenant models.Tenant, orders []shopee.Order) {
	for _, shopeeOrder := range orders {
		var existingOrder models.Order
		// 檢查訂單是否已存在 (根據 platform_order_id)
		err := database.DB.Where("tenant_id = ? AND platform_order_id = ?", tenant.ID, shopeeOrder.OrderSN).First(&existingOrder).Error

		status := mapShopeeStatus(shopeeOrder.OrderStatus)
		deliveryPlatform := "shopee"
		sourceType := "shopee" // 或者 "delivery"，視乎系統約定，這裡暫定 shopee

		if err == nil {
			// 訂單已存在，更新狀態
			if existingOrder.Status != status {
				existingOrder.Status = status
				existingOrder.UpdatedAt = time.Now()
				database.DB.Save(&existingOrder)
				log.Printf("🔄 Updated order %s status to %s", shopeeOrder.OrderSN, status)
			}
			continue
		}

		// 創建新訂單
		newOrder := models.Order{
			TenantID:         tenant.ID,
			OrderNumber:      shopeeOrder.OrderSN, // 使用 Shopee 訂單號作為系統訂單號
			OrderDate:        time.Unix(shopeeOrder.CreateTime, 0),
			Status:           status,
			TotalAmount:      shopeeOrder.TotalAmount,
			SourceType:       sourceType,
			DeliveryPlatform: &deliveryPlatform,
			PlatformOrderID:  &shopeeOrder.OrderSN,
			ContactName:      shopeeOrder.RecipientAddress.Name,
			ContactPhone:     shopeeOrder.RecipientAddress.Phone,
			ContactAddress:   shopeeOrder.RecipientAddress.FullAddress,
			Notes:            shopeeOrder.MessageToSeller,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
			ExtraFields:      models.JSONB{}, // 空 JSONB
		}

		// 處理訂單項目
		var orderItems []models.OrderItem
		for _, item := range shopeeOrder.ItemList {
			sku := item.ModelSKU
			if sku == "" {
				sku = item.ItemSKU
			}

			var productID *uuid.UUID
			var product models.Product

			// 嘗試根據 SKU 查找產品
			if sku != "" {
				if err := database.DB.Where("tenant_id = ? AND sku = ?", tenant.ID, sku).First(&product).Error; err == nil {
					productID = &product.ID
				}
			}

			itemName := item.ItemName
			if item.ModelName != "" {
				itemName += " - " + item.ModelName
			}
			platformItemID := strconv.FormatInt(item.ItemID, 10)
			if item.ModelID != 0 {
				platformItemID += "_" + strconv.FormatInt(item.ModelID, 10)
			}

			// 計算單價 (Shopee 給的是折扣後的價格)
			unitPrice := item.ModelDiscountedPrice
			if unitPrice == 0 {
				unitPrice = item.ModelOriginalPrice
			}

			qty := float64(item.ModelQuantityPurchased)

			orderItem := models.OrderItem{
				TenantID:       tenant.ID,
				ProductID:      productID,
				Quantity:       qty,
				UnitPrice:      unitPrice,
				TotalPrice:     unitPrice * qty,
				PlatformItemID: &platformItemID,
				ItemName:       &itemName,
				CreatedAt:      time.Now(),
				ExtraFields:    models.JSONB{},
			}
			orderItems = append(orderItems, orderItem)
		}

		newOrder.OrderItems = orderItems

		// 保存訂單及項目
		if err := database.DB.Create(&newOrder).Error; err != nil {
			log.Printf("❌ Failed to create order %s: %v", shopeeOrder.OrderSN, err)
		} else {
			log.Printf("✅ Created new order %s from Shopee", shopeeOrder.OrderSN)
		}
	}
}

func mapShopeeStatus(shopeeStatus string) string {
	switch shopeeStatus {
	case shopee.OrderStatusUnpaid:
		return "pending"
	case shopee.OrderStatusReadyToShip, shopee.OrderStatusProcessed, shopee.OrderStatusInvoicePending:
		return "processing"
	case shopee.OrderStatusShipped:
		return "shipped"
	case shopee.OrderStatusCompleted:
		return "completed"
	case shopee.OrderStatusInCancel, shopee.OrderStatusCancelled:
		return "cancelled"
	default:
		return "draft"
	}
}

func getShopeeClient(settings map[string]interface{}) (*shopee.Client, error) {
	appKeyStr := parseString(settings["app_key"], "")
	appSecret := parseString(settings["app_secret"], "")
	shopIDStr := parseString(settings["shop_id"], "")
	accessToken := parseString(settings["access_token"], "")
	refreshToken := parseString(settings["refresh_token"], "")
	region := parseString(settings["region"], "TW")

	partnerID, _ := strconv.ParseInt(appKeyStr, 10, 64)
	shopID, _ := strconv.ParseInt(shopIDStr, 10, 64)

	return shopee.NewClient(partnerID, appSecret, shopID, accessToken, refreshToken, region), nil
}

func parseString(value interface{}, fallback string) string {
	if value == nil {
		return fallback
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fallback
}

// refreshShopeeTokensForAllTenants 為所有需要刷新 token 的租戶刷新 token
func refreshShopeeTokensForAllTenants() {
	// 查詢所有有 Shopee access token 的租戶
	var tenants []models.Tenant
	if err := database.DB.
		Where("extra_fields->'product_sync'->'shopee'->>'access_token' IS NOT NULL").
		Where("extra_fields->'product_sync'->'shopee'->>'access_token' != ''").
		Find(&tenants).Error; err != nil {
		log.Printf("❌ Failed to query tenants for Shopee token refresh: %v", err)
		return
	}

	if len(tenants) == 0 {
		return
	}

	for _, tenant := range tenants {
		settings := tenant.ExtraFields["product_sync"]
		if settings == nil {
			continue
		}

		settingsMap, ok := settings.(map[string]interface{})
		if !ok {
			continue
		}

		shopeeSettings, ok := settingsMap["shopee"].(map[string]interface{})
		if !ok {
			continue
		}

		// 檢查 token 是否即將過期（2小時內）
		tokenExpireTime := int64(0)
		if v, ok := shopeeSettings["token_expire_time"].(float64); ok {
			tokenExpireTime = int64(v)
		} else if v, ok := shopeeSettings["token_expire_time"].(int64); ok {
			tokenExpireTime = v
		}

		// 如果 token 在 2 小時內過期，刷新它
		if time.Now().Unix() > (tokenExpireTime - 7200) {
			client, err := getShopeeClient(shopeeSettings)
			if err != nil {
				continue
			}

			tokenResp, err := client.RefreshAccessToken()
			if err != nil {
				log.Printf("❌ Failed to refresh Shopee token for tenant %s: %v", tenant.ID, err)
				continue
			}

			// 更新儲存的 token
			shopeeSettings["access_token"] = tokenResp.AccessToken
			shopeeSettings["refresh_token"] = tokenResp.RefreshToken
			shopeeSettings["token_expire_time"] = time.Now().Unix() + int64(tokenResp.ExpireIn)
			settingsMap["shopee"] = shopeeSettings
			tenant.ExtraFields["product_sync"] = settingsMap
			database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields)

			log.Printf("🔑 Refreshed Shopee token for tenant %s", tenant.ID)
		}
	}
}
