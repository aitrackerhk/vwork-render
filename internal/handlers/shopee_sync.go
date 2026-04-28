package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/services/shopee"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetShopeeAuthURL 取得 Shopee OAuth 授權 URL
func GetShopeeAuthURL(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 取得 Shopee 設定
	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	appKeyStr := parseStringFromExtra(shopeeSettings["app_key"], "")
	appSecret := parseStringFromExtra(shopeeSettings["app_secret"], "")
	region := parseStringFromExtra(shopeeSettings["region"], "TW")

	if appKeyStr == "" || appSecret == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Shopee App Key 和 App Secret 必須先設定",
		})
	}

	partnerID, err := strconv.ParseInt(appKeyStr, 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "無效的 Partner ID (App Key)",
		})
	}

	// 建立 Shopee client
	client := shopee.NewClient(partnerID, appSecret, 0, "", "", region)

	// 產生 redirect URL
	baseURL := c.Get("Origin")
	if baseURL == "" {
		baseURL = fmt.Sprintf("%s://%s", c.Protocol(), c.Hostname())
	}
	redirectURL := fmt.Sprintf("%s/api/shopee/callback?tenant_id=%s", baseURL, tenantID.String())

	authURL := client.GenerateAuthURL(redirectURL)

	return c.JSON(fiber.Map{
		"auth_url": authURL,
	})
}

// ShopeeCallback 處理 Shopee OAuth 回調
func ShopeeCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	shopIDStr := c.Query("shop_id")
	tenantIDStr := c.Query("tenant_id")

	if code == "" || shopIDStr == "" || tenantIDStr == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Missing required parameters",
		})
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid tenant ID",
		})
	}

	shopID, err := strconv.ParseInt(shopIDStr, 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid shop ID",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 取得 Shopee 設定
	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	appKeyStr := parseStringFromExtra(shopeeSettings["app_key"], "")
	appSecret := parseStringFromExtra(shopeeSettings["app_secret"], "")
	region := parseStringFromExtra(shopeeSettings["region"], "TW")

	if appKeyStr == "" || appSecret == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Shopee settings not configured",
		})
	}

	partnerID, err := strconv.ParseInt(appKeyStr, 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid Partner ID",
		})
	}

	// 建立 Shopee client 並取得 access token
	client := shopee.NewClient(partnerID, appSecret, shopID, "", "", region)
	tokenResp, err := client.GetAccessToken(code, shopID)
	if err != nil {
		log.Printf("Failed to get Shopee access token: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("無法取得 access token: %v", err),
		})
	}

	// 儲存 token 到 tenant settings
	shopeeSettings["shop_id"] = strconv.FormatInt(shopID, 10)
	shopeeSettings["access_token"] = tokenResp.AccessToken
	shopeeSettings["refresh_token"] = tokenResp.RefreshToken
	shopeeSettings["token_expire_time"] = time.Now().Unix() + int64(tokenResp.ExpireIn)
	shopeeSettings["auth_time"] = time.Now().Unix()

	if tenant.ExtraFields == nil {
		tenant.ExtraFields = models.JSONB{}
	}
	settings["shopee"] = shopeeSettings
	tenant.ExtraFields["product_sync"] = settings

	if err := database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to save access token",
		})
	}

	// 重定向回設定頁面
	return c.Redirect("/product-sync-settings?shopee=connected")
}

// RefreshShopeeToken 刷新 Shopee access token
func RefreshShopeeToken(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 取得 Shopee 設定
	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	client, err := getShopeeClientFromSettings(shopeeSettings)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	tokenResp, err := client.RefreshAccessToken()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("無法刷新 token: %v", err),
		})
	}

	// 更新 token
	shopeeSettings["access_token"] = tokenResp.AccessToken
	shopeeSettings["refresh_token"] = tokenResp.RefreshToken
	shopeeSettings["token_expire_time"] = time.Now().Unix() + int64(tokenResp.ExpireIn)

	settings["shopee"] = shopeeSettings
	tenant.ExtraFields["product_sync"] = settings

	if err := database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to save refreshed token",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Token refreshed successfully",
	})
}

// GetShopeeShopInfo 取得 Shopee 店舖資訊
func GetShopeeShopInfo(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	client, err := getShopeeClientFromSettings(shopeeSettings)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	shopInfo, err := client.GetShopInfo()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("無法取得店舖資訊: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"data": shopInfo,
	})
}

// GetShopeeProducts 取得 Shopee 商品列表
func GetShopeeProducts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	client, err := getShopeeClientFromSettings(shopeeSettings)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// 取得商品列表
	status := c.Query("status", "NORMAL") // NORMAL, BANNED, DELETED, UNLIST
	products, err := client.GetAllProducts(status)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("無法取得商品列表: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"data":  products,
		"total": len(products),
	})
}

// GetShopeeOrders 取得 Shopee 訂單列表
func GetShopeeOrders(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	client, err := getShopeeClientFromSettings(shopeeSettings)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// 取得訂單
	daysStr := c.Query("days", "7")
	days, _ := strconv.Atoi(daysStr)
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}

	orderStatus := c.Query("status", "") // 空字串代表全部
	orders, err := client.GetRecentOrders(days, orderStatus)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("無法取得訂單列表: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"data":  orders,
		"total": len(orders),
	})
}

// SyncShopeeInventory 同步庫存到 Shopee
func SyncShopeeInventory(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var req struct {
		ItemID  int64 `json:"item_id"`
		ModelID int64 `json:"model_id,omitempty"`
		Stock   int   `json:"stock"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	client, err := getShopeeClientFromSettings(shopeeSettings)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	stockList := []shopee.StockUpdateItem{
		{
			ModelID:     req.ModelID,
			NormalStock: req.Stock,
		},
	}

	if err := client.UpdateStock(req.ItemID, stockList); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("無法更新庫存: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"message": "庫存更新成功",
	})
}

// SyncShopeePrice 同步價格到 Shopee
func SyncShopeePrice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var req struct {
		ItemID  int64   `json:"item_id"`
		ModelID int64   `json:"model_id,omitempty"`
		Price   float64 `json:"price"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	client, err := getShopeeClientFromSettings(shopeeSettings)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	priceList := []shopee.PriceUpdateItem{
		{
			ModelID:       req.ModelID,
			OriginalPrice: req.Price,
		},
	}

	if err := client.UpdatePrice(req.ItemID, priceList); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("無法更新價格: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"message": "價格更新成功",
	})
}

// TestShopeeConnection 測試 Shopee 連線
func TestShopeeConnection(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	client, err := getShopeeClientFromSettings(shopeeSettings)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"connected": false,
			"error":     err.Error(),
		})
	}

	// 嘗試取得店舖資訊來驗證連線
	shopInfo, err := client.GetShopInfo()
	if err != nil {
		return c.JSON(fiber.Map{
			"connected": false,
			"error":     err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"connected": true,
		"shop_name": shopInfo.ShopName,
		"shop_id":   shopInfo.ShopID,
		"region":    shopInfo.Region,
	})
}

// getShopeeClientFromSettings 從設定建立 Shopee client
func getShopeeClientFromSettings(settings map[string]interface{}) (*shopee.Client, error) {
	appKeyStr := parseStringFromExtra(settings["app_key"], "")
	appSecret := parseStringFromExtra(settings["app_secret"], "")
	shopIDStr := parseStringFromExtra(settings["shop_id"], "")
	accessToken := parseStringFromExtra(settings["access_token"], "")
	refreshToken := parseStringFromExtra(settings["refresh_token"], "")
	region := parseStringFromExtra(settings["region"], "TW")

	if appKeyStr == "" || appSecret == "" {
		return nil, fmt.Errorf("Shopee App Key 和 App Secret 未設定")
	}

	partnerID, err := strconv.ParseInt(appKeyStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("無效的 Partner ID (App Key)")
	}

	var shopID int64
	if shopIDStr != "" {
		shopID, _ = strconv.ParseInt(shopIDStr, 10, 64)
	}

	if accessToken == "" {
		return nil, fmt.Errorf("尚未授權連接 Shopee，請先進行 OAuth 授權")
	}

	return shopee.NewClient(partnerID, appSecret, shopID, accessToken, refreshToken, region), nil
}

// ShopeeSyncStatus 結構用於追蹤同步狀態
type ShopeeSyncStatus struct {
	TenantID        uuid.UUID `json:"tenant_id"`
	LastSyncTime    time.Time `json:"last_sync_time"`
	ProductsSynced  int       `json:"products_synced"`
	OrdersSynced    int       `json:"orders_synced"`
	InventorySynced int       `json:"inventory_synced"`
	PricesSynced    int       `json:"prices_synced"`
	Errors          []string  `json:"errors,omitempty"`
	Status          string    `json:"status"` // "success", "partial", "failed"
}

// RunShopeeSyncForTenant 為指定租戶執行 Shopee 同步
func RunShopeeSyncForTenant(tenantID uuid.UUID) (*ShopeeSyncStatus, error) {
	syncStatus := &ShopeeSyncStatus{
		TenantID:     tenantID,
		LastSyncTime: time.Now(),
		Errors:       []string{},
		Status:       "success",
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		syncStatus.Status = "failed"
		syncStatus.Errors = append(syncStatus.Errors, "Tenant not found")
		return syncStatus, err
	}

	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	enabled := parseBoolFromExtra(shopeeSettings["enabled"], false)
	if !enabled {
		syncStatus.Status = "skipped"
		return syncStatus, nil
	}

	client, err := getShopeeClientFromSettings(shopeeSettings)
	if err != nil {
		syncStatus.Status = "failed"
		syncStatus.Errors = append(syncStatus.Errors, err.Error())
		return syncStatus, err
	}

	// 檢查 token 是否需要刷新
	tokenExpireTime := int64(0)
	if v, ok := shopeeSettings["token_expire_time"].(float64); ok {
		tokenExpireTime = int64(v)
	} else if v, ok := shopeeSettings["token_expire_time"].(int64); ok {
		tokenExpireTime = v
	}

	if shopee.TokenNeedsRefresh(tokenExpireTime) {
		log.Printf("Refreshing Shopee token for tenant %s", tenantID)
		if _, err := client.RefreshAccessToken(); err != nil {
			syncStatus.Errors = append(syncStatus.Errors, fmt.Sprintf("Failed to refresh token: %v", err))
			// 繼續嘗試，token 可能還沒過期
		} else {
			// 更新儲存的 token
			shopeeSettings["access_token"] = client.AccessToken
			shopeeSettings["refresh_token"] = client.RefreshToken
			shopeeSettings["token_expire_time"] = time.Now().Unix() + 14400 // 4 hours
			settings["shopee"] = shopeeSettings
			tenant.ExtraFields["product_sync"] = settings
			database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields)
		}
	}

	syncProducts := parseBoolFromExtra(shopeeSettings["sync_products"], true)
	syncOrders := parseBoolFromExtra(shopeeSettings["sync_orders"], true)

	// 同步商品
	if syncProducts {
		products, err := client.GetAllProducts("NORMAL")
		if err != nil {
			syncStatus.Errors = append(syncStatus.Errors, fmt.Sprintf("Failed to sync products: %v", err))
			syncStatus.Status = "partial"
		} else {
			syncStatus.ProductsSynced = len(products)
			// 將商品資料同步到本地資料庫
			log.Printf("Synced %d products from Shopee for tenant %s", len(products), tenantID)

			for _, p := range products {
				var product models.Product
				// 檢查是否存在 (使用 extra_fields 查詢)
				// 注意：這裡假設 PostgreSQL 的 JSONB 查詢語法
				// 如果使用 SQLite，語法可能不同，需要根據實際 DB 調整。
				// 為了相容性，我們先嘗試用 SKU 查詢，如果沒有 SKU 再用 ID
				// 但 Shopee ItemID 是唯一的，最好存在 ExtraFields

				// 這裡簡化邏輯：先查詢是否已存在該 Shopee ItemID
				var count int64
				err := database.DB.Model(&models.Product{}).
					Where("tenant_id = ? AND extra_fields->>'shopee_item_id' = ?", tenantID, strconv.FormatInt(p.ItemID, 10)).
					Count(&count).Error

				if err == nil && count > 0 {
					// Update
					database.DB.Where("tenant_id = ? AND extra_fields->>'shopee_item_id' = ?", tenantID, strconv.FormatInt(p.ItemID, 10)).First(&product)
				} else {
					// Create
					product = models.Product{
						TenantID: tenantID,
						Status:   "active",
					}
				}

				product.Name = p.ItemName
				product.Description = p.Description
				product.SKU = p.ItemSKU
				if product.SKU == "" {
					product.SKU = fmt.Sprintf("SHOPEE-%d", p.ItemID)
				}

				// 處理價格和庫存
				// 如果有 Model，這裡應該取 Model 的總庫存和價格範圍（或最低價）
				// 暫時取第一個 Model 或基本資訊
				if len(p.PriceInfo) > 0 {
					product.Price = p.PriceInfo[0].CurrentPrice
				}
				if len(p.StockInfo) > 0 {
					product.StockQuantity = p.StockInfo[0].CurrentStock
				}

				// 處理圖片
				if len(p.Image.ImageURLList) > 0 {
					product.ImageURL = p.Image.ImageURLList[0]
				}

				// 更新 ExtraFields
				if product.ExtraFields == nil {
					product.ExtraFields = make(models.JSONB)
				}
				product.ExtraFields["shopee_item_id"] = strconv.FormatInt(p.ItemID, 10)
				product.ExtraFields["source"] = "shopee"

				// Save
				if product.ID == uuid.Nil {
					if err := database.DB.Create(&product).Error; err != nil {
						log.Printf("Failed to create product %s: %v", p.ItemName, err)
						syncStatus.Errors = append(syncStatus.Errors, fmt.Sprintf("Failed to create product %d: %v", p.ItemID, err))
					}
				} else {
					if err := database.DB.Save(&product).Error; err != nil {
						log.Printf("Failed to update product %s: %v", p.ItemName, err)
						syncStatus.Errors = append(syncStatus.Errors, fmt.Sprintf("Failed to update product %d: %v", p.ItemID, err))
					}
				}
			}
		}

		// 同步訂單
		if syncOrders {
			orders, err := client.GetRecentOrders(7, "")
			if err != nil {
				syncStatus.Errors = append(syncStatus.Errors, fmt.Sprintf("Failed to sync orders: %v", err))
				syncStatus.Status = "partial"
			} else {
				syncStatus.OrdersSynced = len(orders)
				// 將訂單資料同步到本地資料庫
				log.Printf("Synced %d orders from Shopee for tenant %s", len(orders), tenantID)

				for _, o := range orders {
					var order models.Order

					// 檢查是否存在
					var count int64
					err := database.DB.Model(&models.Order{}).
						Where("tenant_id = ? AND extra_fields->>'shopee_order_sn' = ?", tenantID, o.OrderSN).
						Count(&count).Error

					if err == nil && count > 0 {
						// Update
						database.DB.Where("tenant_id = ? AND extra_fields->>'shopee_order_sn' = ?", tenantID, o.OrderSN).First(&order)
					} else {
						// Create
						order = models.Order{
							TenantID: tenantID,
						}
					}

					order.OrderNumber = o.OrderSN
					order.OrderDate = time.Unix(o.CreateTime, 0)
					order.TotalAmount = o.TotalAmount
					order.SourceType = "shopee"
					order.DeliveryPlatform = &order.SourceType // using string pointer helper if needed, but SourceType is string. DeliveryPlatform is *string.
					platform := "shopee"
					order.DeliveryPlatform = &platform
					order.PlatformOrderID = &o.OrderSN

					// Map Status
					switch o.OrderStatus {
					case "UNPAID":
						order.Status = "pending"
					case "READY_TO_SHIP", "PROCESSED", "SHIPPED", "INVOICE_PENDING":
						order.Status = "confirmed"
					case "COMPLETED":
						order.Status = "completed"
					case "IN_CANCEL", "CANCELLED":
						order.Status = "cancelled"
					default:
						order.Status = "pending"
					}

					// Customer info (simple mapping)
					order.ContactName = o.RecipientAddress.Name
					order.ContactPhone = o.RecipientAddress.Phone
					order.ContactAddress = o.RecipientAddress.FullAddress

					// Extra Fields
					if order.ExtraFields == nil {
						order.ExtraFields = make(models.JSONB)
					}
					order.ExtraFields["shopee_order_sn"] = o.OrderSN
					order.ExtraFields["shopee_status"] = o.OrderStatus

					// Save Order
					if order.ID == uuid.Nil {
						if err := database.DB.Create(&order).Error; err != nil {
							log.Printf("Failed to create order %s: %v", o.OrderSN, err)
							syncStatus.Errors = append(syncStatus.Errors, fmt.Sprintf("Failed to create order %s: %v", o.OrderSN, err))
							continue
						}
					} else {
						if err := database.DB.Save(&order).Error; err != nil {
							log.Printf("Failed to update order %s: %v", o.OrderSN, err)
							syncStatus.Errors = append(syncStatus.Errors, fmt.Sprintf("Failed to update order %s: %v", o.OrderSN, err))
							continue
						}
					}

					// Sync Order Items
					// First delete existing items if updating (simplest strategy for sync)
					// Or check diff. For simplicity, we can just ensure they exist.
					// But deleting and recreating ensures we match Shopee exactly.
					if count > 0 {
						database.DB.Where("order_id = ?", order.ID).Delete(&models.OrderItem{})
					}

					for _, item := range o.ItemList {
						orderItem := models.OrderItem{
							TenantID:   tenantID,
							OrderID:    order.ID,
							Quantity:   float64(item.ModelQuantityPurchased),
							UnitPrice:  item.ModelDiscountedPrice,
							TotalPrice: item.ModelDiscountedPrice * float64(item.ModelQuantityPurchased),
							ItemName:   &item.ItemName,
						}

						// Try to link to local product if exists
						var localProduct models.Product
						if err := database.DB.Where("tenant_id = ? AND extra_fields->>'shopee_item_id' = ?", tenantID, strconv.FormatInt(item.ItemID, 10)).First(&localProduct).Error; err == nil {
							orderItem.ProductID = &localProduct.ID
						}

						platformItemID := strconv.FormatInt(item.ItemID, 10)
						orderItem.PlatformItemID = &platformItemID

						// Item Options (Model Name)
						if item.ModelName != "" {
							orderItem.ItemOptions = models.JSONB{"model_name": item.ModelName, "model_sku": item.ModelSKU}
						}

						if err := database.DB.Create(&orderItem).Error; err != nil {
							log.Printf("Failed to create order item for order %s: %v", o.OrderSN, err)
							syncStatus.Errors = append(syncStatus.Errors, fmt.Sprintf("Failed to create order item for order %s: %v", o.OrderSN, err))
						}
					}
				}
			}
		}
	}

	if len(syncStatus.Errors) > 0 && syncStatus.Status == "success" {
		syncStatus.Status = "partial"
	}

	// 保存同步狀態
	shopeeSettings["last_sync_time"] = syncStatus.LastSyncTime.Unix()
	shopeeSettings["last_sync_status"] = syncStatus.Status
	settings["shopee"] = shopeeSettings
	tenant.ExtraFields["product_sync"] = settings
	database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields)

	return syncStatus, nil
}

// ManualShopeeSync 手動觸發 Shopee 同步
func ManualShopeeSync(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	syncStatus, err := RunShopeeSyncForTenant(tenantID)
	if err != nil && syncStatus.Status == "failed" {
		return c.Status(500).JSON(fiber.Map{
			"error":  err.Error(),
			"status": syncStatus,
		})
	}

	return c.JSON(fiber.Map{
		"message": "同步完成",
		"status":  syncStatus,
	})
}

// GetShopeeSyncStatus 取得 Shopee 同步狀態
func GetShopeeSyncStatus(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])
	shopeeSettings := settings["shopee"].(map[string]interface{})

	lastSyncTime := int64(0)
	if v, ok := shopeeSettings["last_sync_time"].(float64); ok {
		lastSyncTime = int64(v)
	}
	lastSyncStatus := parseStringFromExtra(shopeeSettings["last_sync_status"], "never")

	tokenExpireTime := int64(0)
	if v, ok := shopeeSettings["token_expire_time"].(float64); ok {
		tokenExpireTime = int64(v)
	}

	authTime := int64(0)
	if v, ok := shopeeSettings["auth_time"].(float64); ok {
		authTime = int64(v)
	}

	accessToken := parseStringFromExtra(shopeeSettings["access_token"], "")

	return c.JSON(fiber.Map{
		"enabled":             parseBoolFromExtra(shopeeSettings["enabled"], false),
		"connected":           accessToken != "",
		"last_sync_time":      lastSyncTime,
		"last_sync_status":    lastSyncStatus,
		"token_expires_at":    tokenExpireTime,
		"token_needs_refresh": shopee.TokenNeedsRefresh(tokenExpireTime),
		"refresh_token_valid": !shopee.IsRefreshTokenExpired(authTime),
	})
}

// Ensure JSON is imported (used in some error handling)
var _ = json.Marshal

// Ensure strings is imported
var _ = strings.TrimSpace
