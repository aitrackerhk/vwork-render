package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// 印花設定 (StampSetting) CRUD
// ============================================

func GetStampSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var settings []models.StampSetting
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.StampSetting{}).Count(&total)

	if err := query.Preload("EarningProducts").Preload("EarningProducts.Product").
		Preload("EarningServices").Preload("EarningServices.Service").
		Preload("RedeemableProducts").Preload("RedeemableProducts.Product").
		Offset(offset).Limit(limit).Order("created_at DESC").Find(&settings).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  settings,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetStampSetting(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var setting models.StampSetting
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("EarningProducts").Preload("EarningProducts.Product").
		Preload("EarningServices").Preload("EarningServices.Service").
		Preload("RedeemableProducts").Preload("RedeemableProducts.Product").
		First(&setting).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Stamp setting not found"})
	}

	return c.JSON(setting)
}

func CreateStampSetting(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	// 定義請求結構以接收 earning_products, earning_services 和 redeemable_products
	type CreateRequest struct {
		models.StampSetting
		EarningProductIDs     []string `json:"earning_products"`
		EarningServiceIDs     []string `json:"earning_services"`
		RedeemableProductIDs  []string `json:"redeemable_products"`
		DefaultStampsRequired int      `json:"default_stamps_required"`
		DefaultDailyLimit     *int     `json:"default_daily_limit"`
	}

	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	req.StampSetting.TenantID = tenantID

	if err := database.DB.Create(&req.StampSetting).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 處理 earning_products 關聯
	if req.EarningProductIDs != nil {
		for _, productID := range req.EarningProductIDs {
			if productID != "" {
				productUUID, err := uuid.Parse(productID)
				if err != nil {
					continue // 跳過無效的 UUID
				}
				earningProduct := models.StampEarningProduct{
					TenantID:       tenantID,
					StampSettingID: req.StampSetting.ID,
					ProductID:      productUUID,
					StampCount:     req.ProductStampCount,
				}
				database.DB.Create(&earningProduct)
			}
		}
	}

	// 處理 earning_services 關聯
	if req.EarningServiceIDs != nil {
		for _, serviceID := range req.EarningServiceIDs {
			if serviceID != "" {
				serviceUUID, err := uuid.Parse(serviceID)
				if err != nil {
					continue // 跳過無效的 UUID
				}
				earningService := models.StampEarningService{
					TenantID:       tenantID,
					StampSettingID: req.StampSetting.ID,
					ServiceID:      serviceUUID,
					StampCount:     req.ServiceStampCount,
				}
				database.DB.Create(&earningService)
			}
		}
	}

	// 處理 redeemable_products 關聯
	if req.RedeemableProductIDs != nil {
		stampsRequired := 10 // 預設值
		if req.DefaultStampsRequired > 0 {
			stampsRequired = req.DefaultStampsRequired
		}
		for _, productID := range req.RedeemableProductIDs {
			if productID != "" {
				productUUID, err := uuid.Parse(productID)
				if err != nil {
					continue // 跳過無效的 UUID
				}
				redeemableProduct := models.StampRedeemableProduct{
					TenantID:       tenantID,
					StampSettingID: req.StampSetting.ID,
					ProductID:      productUUID,
					StampsRequired: stampsRequired,
					DailyLimit:     req.DefaultDailyLimit,
					Status:         "active",
				}
				database.DB.Create(&redeemableProduct)
			}
		}
	}

	// 重新載入完整資料
	var setting models.StampSetting
	database.DB.Preload("EarningProducts").Preload("EarningProducts.Product").
		Preload("EarningServices").Preload("EarningServices.Service").
		Preload("RedeemableProducts").Preload("RedeemableProducts.Product").
		First(&setting, "id = ?", req.StampSetting.ID)

	return c.Status(201).JSON(setting)
}

func UpdateStampSetting(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var setting models.StampSetting

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&setting).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Stamp setting not found"})
	}

	// 定義請求結構以接收 earning_products, earning_services 和 redeemable_products
	type UpdateRequest struct {
		models.StampSetting
		EarningProductIDs     []string `json:"earning_products"`
		EarningServiceIDs     []string `json:"earning_services"`
		RedeemableProductIDs  []string `json:"redeemable_products"`
		DefaultStampsRequired int      `json:"default_stamps_required"`
		DefaultDailyLimit     *int     `json:"default_daily_limit"`
	}

	var req UpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	setting.Name = req.Name
	setting.Description = req.Description
	setting.Status = req.Status
	setting.ProductStampEnabled = req.ProductStampEnabled
	setting.ProductStampCount = req.ProductStampCount
	setting.ProductStampDailyLimit = req.ProductStampDailyLimit
	setting.ServiceStampEnabled = req.ServiceStampEnabled
	setting.ServiceStampCount = req.ServiceStampCount
	setting.ServiceStampDailyLimit = req.ServiceStampDailyLimit
	setting.AmountStampEnabled = req.AmountStampEnabled
	setting.AmountPerStamp = req.AmountPerStamp
	setting.AmountStampDailyLimit = req.AmountStampDailyLimit
	setting.ValidFrom = req.ValidFrom
	setting.ValidTo = req.ValidTo
	setting.ExtraFields = req.ExtraFields

	if err := database.DB.Save(&setting).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 處理 earning_products 關聯
	if req.EarningProductIDs != nil {
		// 刪除現有關聯
		database.DB.Where("tenant_id = ? AND stamp_setting_id = ?", tenantID, id).Delete(&models.StampEarningProduct{})
		// 創建新關聯
		for _, productID := range req.EarningProductIDs {
			if productID != "" {
				productUUID, err := uuid.Parse(productID)
				if err != nil {
					continue // 跳過無效的 UUID
				}
				earningProduct := models.StampEarningProduct{
					TenantID:       tenantID,
					StampSettingID: setting.ID,
					ProductID:      productUUID,
					StampCount:     req.ProductStampCount,
				}
				database.DB.Create(&earningProduct)
			}
		}
	}

	// 處理 earning_services 關聯
	if req.EarningServiceIDs != nil {
		// 刪除現有關聯
		database.DB.Where("tenant_id = ? AND stamp_setting_id = ?", tenantID, id).Delete(&models.StampEarningService{})
		// 創建新關聯
		for _, serviceID := range req.EarningServiceIDs {
			if serviceID != "" {
				serviceUUID, err := uuid.Parse(serviceID)
				if err != nil {
					continue // 跳過無效的 UUID
				}
				earningService := models.StampEarningService{
					TenantID:       tenantID,
					StampSettingID: setting.ID,
					ServiceID:      serviceUUID,
					StampCount:     req.ServiceStampCount,
				}
				database.DB.Create(&earningService)
			}
		}
	}

	// 處理 redeemable_products 關聯
	if req.RedeemableProductIDs != nil {
		// 獲取現有的 redeemable_products 以保留其設定
		var existingRedeemables []models.StampRedeemableProduct
		database.DB.Where("tenant_id = ? AND stamp_setting_id = ?", tenantID, id).Find(&existingRedeemables)
		existingMap := make(map[uuid.UUID]models.StampRedeemableProduct)
		for _, rp := range existingRedeemables {
			existingMap[rp.ProductID] = rp
		}

		// 刪除現有關聯
		database.DB.Where("tenant_id = ? AND stamp_setting_id = ?", tenantID, id).Delete(&models.StampRedeemableProduct{})

		// 創建新關聯
		stampsRequired := 10 // 預設值
		if req.DefaultStampsRequired > 0 {
			stampsRequired = req.DefaultStampsRequired
		}
		for _, productID := range req.RedeemableProductIDs {
			if productID != "" {
				productUUID, err := uuid.Parse(productID)
				if err != nil {
					continue // 跳過無效的 UUID
				}
				// 如果產品已存在，保留其原有設定
				redeemableProduct := models.StampRedeemableProduct{
					TenantID:       tenantID,
					StampSettingID: setting.ID,
					ProductID:      productUUID,
					StampsRequired: stampsRequired,
					DailyLimit:     req.DefaultDailyLimit,
					Status:         "active",
				}
				if existing, ok := existingMap[productUUID]; ok {
					redeemableProduct.StampsRequired = existing.StampsRequired
					redeemableProduct.QuantityLimit = existing.QuantityLimit
					redeemableProduct.DailyLimit = existing.DailyLimit
					redeemableProduct.Status = existing.Status
				}
				database.DB.Create(&redeemableProduct)
			}
		}
	}

	// 重新載入完整資料
	database.DB.Preload("EarningProducts").Preload("EarningProducts.Product").
		Preload("EarningServices").Preload("EarningServices.Service").
		Preload("RedeemableProducts").Preload("RedeemableProducts.Product").
		First(&setting, "id = ?", setting.ID)

	return c.JSON(setting)
}

func DeleteStampSetting(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.StampSetting{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Stamp setting deleted"})
}

// ============================================
// 印花獲取產品 (StampEarningProduct) CRUD
// ============================================

func GetStampEarningProducts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var products []models.StampEarningProduct
	query := database.DB.Where("tenant_id = ?", tenantID)

	if settingID := c.Query("stamp_setting_id"); settingID != "" {
		query = query.Where("stamp_setting_id = ?", settingID)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.StampEarningProduct{}).Count(&total)

	if err := query.Preload("Product").Offset(offset).Limit(limit).Order("created_at DESC").Find(&products).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  products,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func CreateStampEarningProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var product models.StampEarningProduct
	if err := c.BodyParser(&product); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	product.TenantID = tenantID

	// 檢查是否已存在
	var existing models.StampEarningProduct
	if err := database.DB.Where("tenant_id = ? AND stamp_setting_id = ? AND product_id = ?",
		tenantID, product.StampSettingID, product.ProductID).First(&existing).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Product already added to this stamp setting"})
	}

	if err := database.DB.Create(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(product)
}

func DeleteStampEarningProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.StampEarningProduct{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Stamp earning product deleted"})
}

// ============================================
// 印花獲取服務 (StampEarningService) CRUD
// ============================================

func GetStampEarningServices(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var services []models.StampEarningService
	query := database.DB.Where("tenant_id = ?", tenantID)

	if settingID := c.Query("stamp_setting_id"); settingID != "" {
		query = query.Where("stamp_setting_id = ?", settingID)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.StampEarningService{}).Count(&total)

	if err := query.Preload("Service").Offset(offset).Limit(limit).Order("created_at DESC").Find(&services).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  services,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func CreateStampEarningService(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var service models.StampEarningService
	if err := c.BodyParser(&service); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	service.TenantID = tenantID

	// 檢查是否已存在
	var existing models.StampEarningService
	if err := database.DB.Where("tenant_id = ? AND stamp_setting_id = ? AND service_id = ?",
		tenantID, service.StampSettingID, service.ServiceID).First(&existing).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Service already added to this stamp setting"})
	}

	if err := database.DB.Create(&service).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(service)
}

func DeleteStampEarningService(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.StampEarningService{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Stamp earning service deleted"})
}

// ============================================
// 印花可換購產品 (StampRedeemableProduct) CRUD
// ============================================

func GetStampRedeemableProducts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var products []models.StampRedeemableProduct
	query := database.DB.Where("tenant_id = ?", tenantID)

	if settingID := c.Query("stamp_setting_id"); settingID != "" {
		query = query.Where("stamp_setting_id = ?", settingID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.StampRedeemableProduct{}).Count(&total)

	if err := query.Preload("Product").Offset(offset).Limit(limit).Order("created_at DESC").Find(&products).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  products,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetStampRedeemableProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var product models.StampRedeemableProduct
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("Product").First(&product).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Redeemable product not found"})
	}

	return c.JSON(product)
}

func CreateStampRedeemableProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var product models.StampRedeemableProduct
	if err := c.BodyParser(&product); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	product.TenantID = tenantID

	// 檢查是否已存在
	var existing models.StampRedeemableProduct
	if err := database.DB.Where("tenant_id = ? AND stamp_setting_id = ? AND product_id = ?",
		tenantID, product.StampSettingID, product.ProductID).First(&existing).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Product already added as redeemable"})
	}

	if err := database.DB.Create(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(product)
}

func UpdateStampRedeemableProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var product models.StampRedeemableProduct

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&product).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Redeemable product not found"})
	}

	var req models.StampRedeemableProduct
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	product.StampsRequired = req.StampsRequired
	product.QuantityLimit = req.QuantityLimit
	product.DailyLimit = req.DailyLimit
	product.Status = req.Status

	if err := database.DB.Save(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(product)
}

func DeleteStampRedeemableProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.StampRedeemableProduct{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Redeemable product deleted"})
}

// ============================================
// 印花記錄 (StampRecord) CRUD
// ============================================

func GetStampRecords(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var records []models.StampRecord
	query := database.DB.Where("tenant_id = ?", tenantID)

	if customerID := c.Query("customer_id"); customerID != "" {
		query = query.Where("customer_id = ?", customerID)
	}
	if settingID := c.Query("stamp_setting_id"); settingID != "" {
		query = query.Where("stamp_setting_id = ?", settingID)
	}
	if recordType := c.Query("record_type"); recordType != "" {
		query = query.Where("record_type = ?", recordType)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.StampRecord{}).Count(&total)

	if err := query.Preload("Customer").Preload("StampSetting").Preload("Product").
		Offset(offset).Limit(limit).Order("created_at DESC").Find(&records).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  records,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetStampRecord(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var record models.StampRecord
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("Customer").Preload("StampSetting").Preload("Product").First(&record).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Stamp record not found"})
	}

	return c.JSON(record)
}

// CreateStampRecord 手動增減印花
func CreateStampRecord(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		CustomerID     uuid.UUID  `json:"customer_id"`
		StampSettingID *uuid.UUID `json:"stamp_setting_id"`
		RecordType     string     `json:"record_type"` // earn, redeem
		StampCount     int        `json:"stamp_count"`
		Notes          string     `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.CustomerID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Customer ID is required"})
	}
	if req.StampCount == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Stamp count cannot be zero"})
	}

	// 獲取或創建餘額記錄
	var balance models.CustomerStampBalance
	if req.StampSettingID != nil {
		if err := database.DB.Where("tenant_id = ? AND customer_id = ? AND stamp_setting_id = ?",
			tenantID, req.CustomerID, *req.StampSettingID).First(&balance).Error; err != nil {
			// 創建新的餘額記錄
			balance = models.CustomerStampBalance{
				TenantID:       tenantID,
				CustomerID:     req.CustomerID,
				StampSettingID: *req.StampSettingID,
				Balance:        0,
			}
			database.DB.Create(&balance)
		}
	}

	// 計算新餘額
	stampCount := req.StampCount
	if req.RecordType == "redeem" && stampCount > 0 {
		stampCount = -stampCount // 兌換時為負數
	}
	newBalance := balance.Balance + stampCount

	if newBalance < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Insufficient stamps"})
	}

	// 創建記錄
	record := models.StampRecord{
		TenantID:       tenantID,
		CustomerID:     req.CustomerID,
		StampSettingID: req.StampSettingID,
		RecordType:     req.RecordType,
		StampCount:     stampCount,
		BalanceAfter:   newBalance,
		SourceType:     "manual",
		Notes:          req.Notes,
		CreatedBy:      &userID,
	}

	if err := database.DB.Create(&record).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新餘額
	now := time.Now()
	balance.Balance = newBalance
	if stampCount > 0 {
		balance.TotalEarned += stampCount
		balance.LastEarnedAt = &now
	} else {
		balance.TotalRedeemed += -stampCount
		balance.LastRedeemedAt = &now
	}
	database.DB.Save(&balance)

	return c.Status(201).JSON(record)
}

// ============================================
// 客戶印花餘額 API
// ============================================

func GetCustomerStampBalances(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var balances []models.CustomerStampBalance
	query := database.DB.Where("tenant_id = ?", tenantID)

	if customerID := c.Query("customer_id"); customerID != "" {
		query = query.Where("customer_id = ?", customerID)
	}
	if settingID := c.Query("stamp_setting_id"); settingID != "" {
		query = query.Where("stamp_setting_id = ?", settingID)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.CustomerStampBalance{}).Count(&total)

	if err := query.Preload("Customer").Preload("StampSetting").
		Offset(offset).Limit(limit).Order("updated_at DESC").Find(&balances).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  balances,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetCustomerStamps 獲取客戶所有印花餘額 (用於訂單/POS)
func GetCustomerStamps(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID := c.Query("customer_id")

	if customerID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Customer ID is required"})
	}

	var balances []models.CustomerStampBalance
	if err := database.DB.Where("tenant_id = ? AND customer_id = ? AND balance > 0", tenantID, customerID).
		Preload("StampSetting").Find(&balances).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": balances})
}

// GetAvailableRedeemableProducts 獲取可兌換產品列表
func GetAvailableRedeemableProducts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID := c.Query("customer_id")

	// 獲取活躍的印花設定
	var settings []models.StampSetting
	now := time.Now()
	query := database.DB.Where("tenant_id = ? AND status = 'active'", tenantID).
		Where("(valid_from IS NULL OR valid_from <= ?) AND (valid_to IS NULL OR valid_to >= ?)", now, now)

	if err := query.Preload("RedeemableProducts", "status = 'active'").
		Preload("RedeemableProducts.Product").Find(&settings).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 如果有客戶ID，獲取其印花餘額
	var balanceMap = make(map[uuid.UUID]int)
	if customerID != "" {
		var balances []models.CustomerStampBalance
		database.DB.Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).Find(&balances)
		for _, b := range balances {
			balanceMap[b.StampSettingID] = b.Balance
		}
	}

	// 構建返回數據
	var result []fiber.Map
	for _, setting := range settings {
		customerBalance := balanceMap[setting.ID]
		for _, rp := range setting.RedeemableProducts {
			canRedeem := customerBalance >= rp.StampsRequired
			result = append(result, fiber.Map{
				"id":               rp.ID,
				"stamp_setting_id": setting.ID,
				"stamp_setting":    setting.Name,
				"product_id":       rp.ProductID,
				"product":          rp.Product,
				"stamps_required":  rp.StampsRequired,
				"customer_balance": customerBalance,
				"can_redeem":       canRedeem,
			})
		}
	}

	return c.JSON(fiber.Map{"data": result})
}

// RedeemStamps 使用印花兌換產品
func RedeemStamps(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		CustomerID          uuid.UUID `json:"customer_id"`
		RedeemableProductID uuid.UUID `json:"redeemable_product_id"`
		Quantity            int       `json:"quantity"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Quantity <= 0 {
		req.Quantity = 1
	}

	// 獲取可兌換產品
	var redeemable models.StampRedeemableProduct
	if err := database.DB.Where("tenant_id = ? AND id = ? AND status = 'active'", tenantID, req.RedeemableProductID).
		Preload("Product").First(&redeemable).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Redeemable product not found or inactive"})
	}

	// 檢查數量限制
	if redeemable.QuantityLimit != nil && req.Quantity > *redeemable.QuantityLimit {
		return c.Status(400).JSON(fiber.Map{"error": "Quantity exceeds limit"})
	}

	// 計算所需印花
	stampsNeeded := redeemable.StampsRequired * req.Quantity

	// 獲取客戶印花餘額
	var balance models.CustomerStampBalance
	if err := database.DB.Where("tenant_id = ? AND customer_id = ? AND stamp_setting_id = ?",
		tenantID, req.CustomerID, redeemable.StampSettingID).First(&balance).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Customer has no stamps for this setting"})
	}

	if balance.Balance < stampsNeeded {
		return c.Status(400).JSON(fiber.Map{"error": "Insufficient stamps"})
	}

	// 檢查每日限制
	if redeemable.DailyLimit != nil {
		today := time.Now().Format("2006-01-02")
		var dailyCount int64
		database.DB.Model(&models.StampDailyRecord{}).
			Where("tenant_id = ? AND customer_id = ? AND stamp_setting_id = ? AND redeemed_product_id = ? AND record_date = ?",
				tenantID, req.CustomerID, redeemable.StampSettingID, redeemable.ProductID, today).
			Select("COALESCE(SUM(redeemed_count), 0)").Scan(&dailyCount)

		if int(dailyCount)+req.Quantity > *redeemable.DailyLimit {
			return c.Status(400).JSON(fiber.Map{"error": "Daily redemption limit exceeded"})
		}
	}

	// 扣減印花
	newBalance := balance.Balance - stampsNeeded
	now := time.Now()

	// 創建記錄
	record := models.StampRecord{
		TenantID:       tenantID,
		CustomerID:     req.CustomerID,
		StampSettingID: &redeemable.StampSettingID,
		RecordType:     "redeem",
		StampCount:     -stampsNeeded,
		BalanceAfter:   newBalance,
		SourceType:     "redeem",
		ProductID:      &redeemable.ProductID,
		Notes:          "兌換產品: " + redeemable.Product.Name,
		CreatedBy:      &userID,
	}

	if err := database.DB.Create(&record).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新餘額
	balance.Balance = newBalance
	balance.TotalRedeemed += stampsNeeded
	balance.LastRedeemedAt = &now
	database.DB.Save(&balance)

	// 更新每日記錄
	today := time.Now()
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	var dailyRecord models.StampDailyRecord
	if err := database.DB.Where("tenant_id = ? AND customer_id = ? AND stamp_setting_id = ? AND redeemed_product_id = ? AND record_date = ?",
		tenantID, req.CustomerID, redeemable.StampSettingID, redeemable.ProductID, todayDate).First(&dailyRecord).Error; err != nil {
		dailyRecord = models.StampDailyRecord{
			TenantID:          tenantID,
			CustomerID:        req.CustomerID,
			StampSettingID:    redeemable.StampSettingID,
			RecordDate:        todayDate,
			RedeemedProductID: &redeemable.ProductID,
			RedeemedCount:     req.Quantity,
		}
		database.DB.Create(&dailyRecord)
	} else {
		dailyRecord.RedeemedCount += req.Quantity
		database.DB.Save(&dailyRecord)
	}

	return c.JSON(fiber.Map{
		"message":       "Stamps redeemed successfully",
		"product":       redeemable.Product,
		"quantity":      req.Quantity,
		"stamps_used":   stampsNeeded,
		"balance_after": newBalance,
		"record":        record,
	})
}

// EarnStampsFromOrder 從訂單獲得印花 (內部函數，由訂單處理調用)
func EarnStampsFromOrder(tenantID uuid.UUID, customerID uuid.UUID, orderID uuid.UUID, orderItems []models.OrderItem, totalAmount float64, userID *uuid.UUID) error {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// 獲取活躍的印花設定
	var settings []models.StampSetting
	if err := database.DB.Where("tenant_id = ? AND status = 'active'", tenantID).
		Where("(valid_from IS NULL OR valid_from <= ?) AND (valid_to IS NULL OR valid_to >= ?)", now, now).
		Preload("EarningProducts").Find(&settings).Error; err != nil {
		return err
	}

	for _, setting := range settings {
		var totalStamps int

		// 1. 產品印花
		if setting.ProductStampEnabled {
			// 建立產品ID到印花數的映射
			productStampMap := make(map[uuid.UUID]int)
			for _, ep := range setting.EarningProducts {
				productStampMap[ep.ProductID] = ep.StampCount
			}

			for _, item := range orderItems {
				if item.ProductID == nil {
					continue
				}
				stampCount, ok := productStampMap[*item.ProductID]
				if !ok {
					continue
				}

				earnedStamps := stampCount * int(item.Quantity)

				// 檢查每日限制
				if setting.ProductStampDailyLimit != nil {
					var dailyRecord models.StampDailyRecord
					if err := database.DB.Where("tenant_id = ? AND customer_id = ? AND stamp_setting_id = ? AND product_id = ? AND record_date = ?",
						tenantID, customerID, setting.ID, *item.ProductID, today).First(&dailyRecord).Error; err == nil {
						remaining := *setting.ProductStampDailyLimit - dailyRecord.ProductStampsEarned
						if remaining <= 0 {
							continue
						}
						if earnedStamps > remaining {
							earnedStamps = remaining
						}
						dailyRecord.ProductStampsEarned += earnedStamps
						database.DB.Save(&dailyRecord)
					} else {
						if earnedStamps > *setting.ProductStampDailyLimit {
							earnedStamps = *setting.ProductStampDailyLimit
						}
						dailyRecord = models.StampDailyRecord{
							TenantID:            tenantID,
							CustomerID:          customerID,
							StampSettingID:      setting.ID,
							RecordDate:          today,
							ProductID:           item.ProductID,
							ProductStampsEarned: earnedStamps,
						}
						database.DB.Create(&dailyRecord)
					}
				}

				totalStamps += earnedStamps
			}
		}

		// 2. 金額印花
		if setting.AmountStampEnabled && setting.AmountPerStamp > 0 {
			earnedStamps := int(totalAmount / setting.AmountPerStamp)

			if earnedStamps > 0 {
				// 檢查每日限制
				if setting.AmountStampDailyLimit != nil {
					var dailyRecord models.StampDailyRecord
					if err := database.DB.Where("tenant_id = ? AND customer_id = ? AND stamp_setting_id = ? AND product_id IS NULL AND record_date = ?",
						tenantID, customerID, setting.ID, today).First(&dailyRecord).Error; err == nil {
						remaining := *setting.AmountStampDailyLimit - dailyRecord.AmountStampsEarned
						if remaining <= 0 {
							earnedStamps = 0
						} else if earnedStamps > remaining {
							earnedStamps = remaining
						}
						if earnedStamps > 0 {
							dailyRecord.AmountStampsEarned += earnedStamps
							database.DB.Save(&dailyRecord)
						}
					} else {
						if earnedStamps > *setting.AmountStampDailyLimit {
							earnedStamps = *setting.AmountStampDailyLimit
						}
						dailyRecord = models.StampDailyRecord{
							TenantID:           tenantID,
							CustomerID:         customerID,
							StampSettingID:     setting.ID,
							RecordDate:         today,
							AmountStampsEarned: earnedStamps,
						}
						database.DB.Create(&dailyRecord)
					}
				}

				totalStamps += earnedStamps
			}
		}

		// 如果有獲得印花，創建記錄
		if totalStamps > 0 {
			// 獲取或創建餘額
			var balance models.CustomerStampBalance
			if err := database.DB.Where("tenant_id = ? AND customer_id = ? AND stamp_setting_id = ?",
				tenantID, customerID, setting.ID).First(&balance).Error; err != nil {
				balance = models.CustomerStampBalance{
					TenantID:       tenantID,
					CustomerID:     customerID,
					StampSettingID: setting.ID,
					Balance:        0,
				}
				database.DB.Create(&balance)
			}

			newBalance := balance.Balance + totalStamps

			// 創建記錄
			record := models.StampRecord{
				TenantID:       tenantID,
				CustomerID:     customerID,
				StampSettingID: &setting.ID,
				RecordType:     "earn",
				StampCount:     totalStamps,
				BalanceAfter:   newBalance,
				SourceType:     "order",
				SourceID:       &orderID,
				Notes:          "訂單獲得印花",
				CreatedBy:      userID,
			}
			database.DB.Create(&record)

			// 更新餘額
			balance.Balance = newBalance
			balance.TotalEarned += totalStamps
			balance.LastEarnedAt = &now
			database.DB.Save(&balance)
		}
	}

	return nil
}
