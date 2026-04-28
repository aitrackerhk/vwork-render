package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/services/amazon"
	"nwork/internal/services/lazada"
	"nwork/internal/services/rakuten"
	"nwork/internal/services/shopee"
	"nwork/internal/utils"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// GetProducts 獲取產品列表
func GetProducts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var products []models.Product
	query := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)

	// 搜索過濾
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ? OR description ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 產品類型過濾
	if typeIDs := parseUUIDListFromStrings(c.Query("product_type_id"), c.Query("product_type_ids")); len(typeIDs) > 0 {
		query = query.Where("product_type_id IN ?", typeIDs)
	}

	// 分類過濾
	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}

	// 標籤過濾
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN product_label_relations ON products.id = product_label_relations.product_id").
			Where("product_label_relations.label_id IN ?", labelIDs).
			Group("products.id")
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Product{}).Count(&total)

	if err := query.Preload("ProductType").Preload("Brand").
		Preload("DefaultSupplier").
		Preload("DefaultWarehouse").
		Preload("ProductTaxes").
		Preload("Labels").
		Offset(offset).Limit(limit).Order("created_at DESC").Find(&products).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch products: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  products,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetProductCategories 取得產品分類清單（去重）
func GetProductCategories(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}
	var categories []string
	if err := database.DB.Model(&models.Product{}).
		Where("tenant_id = ? AND category IS NOT NULL AND category <> ''", tenantID).
		Distinct("category").
		Order("category ASC").
		Pluck("category", &categories).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch categories: " + err.Error()})
	}
	return c.JSON(fiber.Map{
		"data":  categories,
		"total": len(categories),
	})
}

// GetProduct 獲取單個產品
func GetProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid product ID"})
	}

	var product models.Product
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("ProductType").
		Preload("Brand").
		Preload("DefaultSupplier").
		Preload("DefaultWarehouse").
		Preload("ProductTaxes").
		Preload("Labels").
		First(&product).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product not found"})
	}

	// 載入產品屬性值
	var attributeValues []models.ProductAttributeValue
	database.DB.Where("product_id = ?", product.ID).
		Preload("Attribute").
		Find(&attributeValues)

	// 將屬性值添加到響應中
	productMap := make(map[string]interface{})
	productBytes, _ := json.Marshal(product)
	json.Unmarshal(productBytes, &productMap)

	// 產品稅 IDs（給 select2-multi 用）
	productTaxIDs := make([]string, 0)
	if product.ProductTaxes != nil {
		for _, t := range product.ProductTaxes {
			if t.ID != uuid.Nil {
				productTaxIDs = append(productTaxIDs, t.ID.String())
			}
		}
	}
	productMap["product_tax_ids"] = productTaxIDs

	// 產品標籤 IDs（給 select2-multi 用）
	productLabelIDs := make([]string, 0)
	if product.Labels != nil {
		for _, l := range product.Labels {
			if l.ID != uuid.Nil {
				productLabelIDs = append(productLabelIDs, l.ID.String())
			}
		}
	}
	productMap["label_ids"] = productLabelIDs

	// 多選產品類型（從 extra_fields 取值，沒有則使用單一 product_type_id）
	if product.ExtraFields != nil {
		if v, ok := product.ExtraFields["product_type_ids"]; ok {
			productMap["product_type_ids"] = v
		} else if product.ProductTypeID != nil && *product.ProductTypeID != uuid.Nil {
			productMap["product_type_ids"] = []string{product.ProductTypeID.String()}
		}
	} else if product.ProductTypeID != nil && *product.ProductTypeID != uuid.Nil {
		productMap["product_type_ids"] = []string{product.ProductTypeID.String()}
	}

	// 將屬性值轉換為前端需要的格式
	productAttributeValues := make([]map[string]interface{}, 0)
	for _, av := range attributeValues {
		productAttributeValues = append(productAttributeValues, map[string]interface{}{
			"attribute_id": av.AttributeID,
			"value":        av.Value,
		})
	}
	productMap["product_attribute_values"] = productAttributeValues

	return c.JSON(productMap)
}

// CreateProduct 創建產品
func CreateProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Code                   string     `json:"code"`
		SKU                    string     `json:"sku"`
		Barcode                string     `json:"barcode"`
		Name                   string     `json:"name"`
		Description            string     `json:"description"`
		Category               string     `json:"category"`
		SubstanceCategory      string     `json:"substance_category"`
		ProductTypeID          *uuid.UUID `json:"product_type_id"`
		ProductTypeIDs         []string   `json:"product_type_ids"`
		BrandID                *uuid.UUID `json:"brand_id"`
		DefaultSupplierID      *uuid.UUID `json:"default_supplier_id"`
		DefaultWarehouseID     *uuid.UUID `json:"default_warehouse_id"`
		DefaultWarehouseZoneID *uuid.UUID `json:"default_warehouse_zone_id"`
		ProductAttributes      []struct {
			AttributeID uuid.UUID `json:"attribute_id"`
			Value       string    `json:"value"`
		} `json:"product_attributes"`
		ImageURL                string                 `json:"image_url"`
		Price                   float64                `json:"price"`
		Cost                    float64                `json:"cost"`
		StockQuantity           int                    `json:"stock_quantity"`
		Unit                    string                 `json:"unit"`
		IsServicePackage        interface{}            `json:"is_service_package"` // bool or string "true"/"false"
		ServicePackageServiceID *uuid.UUID             `json:"service_package_service_id"`
		IsNonInventory          interface{}            `json:"is_non_inventory"` // bool or string "true"/"false"
		AllowBackorder          interface{}            `json:"allow_backorder"`  // bool or string "true"/"false"
		Status                  string                 `json:"status"`
		ProductTaxIDs           []uuid.UUID            `json:"product_tax_ids"`
		LabelIDs                []uuid.UUID            `json:"label_ids"`
		ExtraFields             map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	if req.Status == "" {
		req.Status = "active"
	}

	now := time.Now()

	// 處理 is_service_package：支持 bool 或 string "true"/"false"
	var isServicePackage bool
	if req.IsServicePackage != nil {
		switch v := req.IsServicePackage.(type) {
		case bool:
			isServicePackage = v
		case string:
			isServicePackage = v == "true" || v == "yes"
		default:
			isServicePackage = false
		}
	}

	// 處理 allow_backorder：支持 bool 或 string "true"/"false"
	var allowBackorder bool
	if req.AllowBackorder != nil {
		switch v := req.AllowBackorder.(type) {
		case bool:
			allowBackorder = v
		case string:
			allowBackorder = v == "true" || v == "yes"
		default:
			allowBackorder = false
		}
	}

	// 處理 is_non_inventory：支持 bool 或 string "true"/"false"
	var isNonInventory bool
	if req.IsNonInventory != nil {
		switch v := req.IsNonInventory.(type) {
		case bool:
			isNonInventory = v
		case string:
			isNonInventory = v == "true" || v == "yes"
		default:
			isNonInventory = false
		}
	}

	// 處理多選產品類型
	var validTypeIDs []string
	if len(req.ProductTypeIDs) > 0 {
		for _, idStr := range req.ProductTypeIDs {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			if _, err := uuid.Parse(idStr); err == nil {
				validTypeIDs = append(validTypeIDs, idStr)
			}
		}
	}
	var primaryTypeID *uuid.UUID
	if len(validTypeIDs) > 0 {
		if parsed, err := uuid.Parse(validTypeIDs[0]); err == nil {
			primaryTypeID = &parsed
		}
	} else {
		primaryTypeID = req.ProductTypeID
	}

	if req.ExtraFields == nil {
		req.ExtraFields = map[string]interface{}{}
	}
	if len(validTypeIDs) > 0 {
		req.ExtraFields["product_type_ids"] = validTypeIDs
	}
	if _, ok := req.ExtraFields["show_on_vmarket"]; !ok {
		if joined, err := getTenantVMarketJoined(tenantID); err == nil && joined {
			req.ExtraFields["show_on_vmarket"] = true
		}
	}

	// 自動生成產品編號（如果未提供）
	autoCode, err := generateAutoCode(tenantID, req.Code, autoCodeConfig{
		Prefix:     "PROD-",
		FieldName:  "code",
		PageName:   "products",
		Format:     codeFormatDate,
		TableModel: &models.Product{},
		Column:     "code",
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate product code: " + err.Error()})
	}

	product := models.Product{
		TenantID:                tenantID,
		Code:                    autoCode,
		SKU:                     req.SKU,
		Barcode:                 req.Barcode,
		Name:                    req.Name,
		Description:             req.Description,
		Category:                req.Category,
		SubstanceCategory:       req.SubstanceCategory,
		ProductTypeID:           primaryTypeID,
		BrandID:                 req.BrandID,
		DefaultSupplierID:       req.DefaultSupplierID,
		DefaultWarehouseID:      req.DefaultWarehouseID,
		DefaultWarehouseZoneID:  req.DefaultWarehouseZoneID,
		ImageURL:                req.ImageURL,
		Price:                   req.Price,
		Cost:                    req.Cost,
		StockQuantity:           req.StockQuantity,
		Unit:                    req.Unit,
		IsServicePackage:        isServicePackage,
		ServicePackageServiceID: req.ServicePackageServiceID,
		IsNonInventory:          isNonInventory,
		AllowBackorder:          allowBackorder,
		Status:                  req.Status,
		CreatedBy:               &userID,
		UpdatedBy:               &userID,
		CreatedAt:               now,
		UpdatedAt:               now,
		ExtraFields:             models.JSONB(req.ExtraFields),
	}

	if err := database.DB.Create(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create product: " + err.Error()})
	}

	// 成功建立後釋放預留編號
	releaseReservedCode(tenantID, "code", product.Code)

	// 綁定產品稅
	if len(req.ProductTaxIDs) > 0 {
		for _, taxID := range req.ProductTaxIDs {
			if taxID == uuid.Nil {
				continue
			}
			_ = database.DB.Create(&models.ProductTaxRelation{
				ProductID: product.ID,
				TaxID:     taxID,
				CreatedAt: now,
			}).Error
		}
	}

	// 綁定產品標籤
	if len(req.LabelIDs) > 0 {
		for _, labelID := range req.LabelIDs {
			if labelID == uuid.Nil {
				continue
			}
			_ = database.DB.Create(&models.ProductLabelRelation{
				ProductID: product.ID,
				LabelID:   labelID,
				CreatedAt: now,
			}).Error
		}
	}

	// 處理產品屬性關聯
	if len(req.ProductAttributes) > 0 {
		for _, attr := range req.ProductAttributes {
			// 驗證 AttributeID 是否為有效的 UUID
			if attr.AttributeID == uuid.Nil {
				fmt.Printf("Invalid attribute ID, skipping: %v\n", attr)
				continue
			}

			// 使用 ON CONFLICT 處理重複的屬性（如果已存在則更新）
			attrValue := models.ProductAttributeValue{
				ProductID:   product.ID,
				AttributeID: attr.AttributeID,
				Value:       attr.Value,
				Status:      "active",
			}

			// 先嘗試查找現有記錄
			var existing models.ProductAttributeValue
			if err := database.DB.Where("product_id = ? AND attribute_id = ?", product.ID, attr.AttributeID).First(&existing).Error; err == nil {
				// 更新現有記錄
				existing.Value = attr.Value
				existing.Status = "active"
				if err := database.DB.Save(&existing).Error; err != nil {
					fmt.Printf("Failed to update product attribute value: %v\n", err)
				}
			} else {
				// 創建新記錄
				if err := database.DB.Create(&attrValue).Error; err != nil {
					// 記錄錯誤但不中斷流程
					fmt.Printf("Failed to create product attribute value: %v\n", err)
				}
			}
		}
	}

	// 觸發 Shopee 同步（非阻塞）
	go shopee.Sync.SyncProductToShopee(tenantID, product.ID, "create")
	// 觸發 Amazon 同步（非阻塞）
	go amazon.Sync.SyncProductToAmazon(tenantID, product.ID)
	// 觸發 Lazada 同步（非阻塞）
	go lazada.Sync.SyncProductToLazada(tenantID, product.ID)
	// 觸發 Rakuten 同步（非阻塞）
	go func() {
		syncService, err := rakuten.NewSyncService(tenantID)
		if err != nil {
			log.Printf("[Rakuten] Failed to create sync service for tenant %s: %v", tenantID, err)
			return
		}
		price := int(product.Price)
		stock := 0
		if err := syncService.SyncPriceAndInventory(product.ID, price, stock); err != nil {
			log.Printf("[Rakuten] Product sync failed for product %s: %v", product.ID, err)
		}
	}()

	return c.Status(201).JSON(product)
}

// UpdateProduct 更新產品
func UpdateProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid product ID"})
	}

	var product models.Product
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&product).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product not found"})
	}

	var req struct {
		Code                   *string    `json:"code"`
		SKU                    *string    `json:"sku"`
		Barcode                *string    `json:"barcode"`
		Name                   *string    `json:"name"`
		Description            *string    `json:"description"`
		Category               *string    `json:"category"`
		SubstanceCategory      *string    `json:"substance_category"`
		ProductTypeID          *uuid.UUID `json:"product_type_id"`
		ProductTypeIDs         []string   `json:"product_type_ids"`
		BrandID                *uuid.UUID `json:"brand_id"`
		DefaultSupplierID      *uuid.UUID `json:"default_supplier_id"`
		DefaultWarehouseID     *uuid.UUID `json:"default_warehouse_id"`
		DefaultWarehouseZoneID *uuid.UUID `json:"default_warehouse_zone_id"`
		ProductAttributes      *[]struct {
			AttributeID uuid.UUID `json:"attribute_id"`
			Value       string    `json:"value"`
		} `json:"product_attributes"`
		ImageURL                *string                 `json:"image_url"`
		Price                   *float64                `json:"price"`
		Cost                    *float64                `json:"cost"`
		StockQuantity           *int                    `json:"stock_quantity"`
		Unit                    *string                 `json:"unit"`
		IsServicePackage        interface{}             `json:"is_service_package"` // bool or string "true"/"false"
		ServicePackageServiceID *uuid.UUID              `json:"service_package_service_id"`
		IsNonInventory          interface{}             `json:"is_non_inventory"` // bool or string "true"/"false"
		AllowBackorder          interface{}             `json:"allow_backorder"`  // bool or string "true"/"false"
		Status                  *string                 `json:"status"`
		ProductTaxIDs           []uuid.UUID             `json:"product_tax_ids"`
		LabelIDs                []uuid.UUID             `json:"label_ids"`
		ExtraFields             *map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Code != nil {
		product.Code = *req.Code
	}
	if req.Name != nil {
		product.Name = *req.Name
	}
	if req.SKU != nil {
		product.SKU = *req.SKU
	}
	if req.Barcode != nil {
		product.Barcode = *req.Barcode
	}
	if req.Description != nil {
		product.Description = *req.Description
	}
	if req.Category != nil {
		product.Category = *req.Category
	}
	if req.SubstanceCategory != nil {
		product.SubstanceCategory = *req.SubstanceCategory
	}
	if req.ProductTypeID != nil {
		product.ProductTypeID = req.ProductTypeID
	}
	if req.BrandID != nil {
		product.BrandID = req.BrandID
	}
	if req.DefaultSupplierID != nil {
		product.DefaultSupplierID = req.DefaultSupplierID
	}
	if req.DefaultWarehouseID != nil {
		product.DefaultWarehouseID = req.DefaultWarehouseID
	}
	if req.DefaultWarehouseZoneID != nil {
		product.DefaultWarehouseZoneID = req.DefaultWarehouseZoneID
	}
	if req.ImageURL != nil {
		product.ImageURL = *req.ImageURL
	}
	// 處理 Price：支持 null 值來清除字段（設為 0）
	if req.Price != nil {
		product.Price = *req.Price
	}
	if req.Cost != nil {
		product.Cost = *req.Cost
	}
	if req.StockQuantity != nil {
		product.StockQuantity = *req.StockQuantity
	}
	if req.Unit != nil {
		product.Unit = *req.Unit
	}
	if req.IsServicePackage != nil {
		switch v := req.IsServicePackage.(type) {
		case bool:
			product.IsServicePackage = v
		case string:
			product.IsServicePackage = v == "true" || v == "yes"
		default:
			// 保持原值
		}
	}
	if req.ServicePackageServiceID != nil {
		product.ServicePackageServiceID = req.ServicePackageServiceID
	} else if req.IsServicePackage != nil {
		// 如果不是服務套票，清空對應服務
		var isServicePackage bool
		switch v := req.IsServicePackage.(type) {
		case bool:
			isServicePackage = v
		case string:
			isServicePackage = v == "true" || v == "yes"
		}
		if !isServicePackage {
			product.ServicePackageServiceID = nil
		}
	}
	if req.AllowBackorder != nil {
		switch v := req.AllowBackorder.(type) {
		case bool:
			product.AllowBackorder = v
		case string:
			product.AllowBackorder = v == "true" || v == "yes"
		default:
			// 保持原值
		}
	}
	if req.IsNonInventory != nil {
		switch v := req.IsNonInventory.(type) {
		case bool:
			product.IsNonInventory = v
		case string:
			product.IsNonInventory = v == "true" || v == "yes"
		default:
			// 保持原值
		}
	}
	if req.Status != nil {
		product.Status = *req.Status
	}
	if req.ExtraFields != nil {
		extraFields := *req.ExtraFields
		if _, ok := extraFields["show_on_vmarket"]; !ok {
			if product.ExtraFields != nil {
				if existing, ok := product.ExtraFields["show_on_vmarket"]; ok {
					extraFields["show_on_vmarket"] = existing
				}
			}
			if _, ok := extraFields["show_on_vmarket"]; !ok {
				if joined, err := getTenantVMarketJoined(tenantID); err == nil && joined {
					extraFields["show_on_vmarket"] = true
				}
			}
		}
		product.ExtraFields = models.JSONB(extraFields)
	}
	// 處理多選產品類型
	if len(req.ProductTypeIDs) > 0 {
		validTypeIDs := make([]string, 0, len(req.ProductTypeIDs))
		for _, idStr := range req.ProductTypeIDs {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			if _, err := uuid.Parse(idStr); err == nil {
				validTypeIDs = append(validTypeIDs, idStr)
			}
		}
		if product.ExtraFields == nil {
			product.ExtraFields = models.JSONB{}
		}
		product.ExtraFields["product_type_ids"] = validTypeIDs
		if len(validTypeIDs) > 0 {
			if parsed, err := uuid.Parse(validTypeIDs[0]); err == nil {
				product.ProductTypeID = &parsed
			}
		}
	}

	product.UpdatedBy = &userID
	product.UpdatedAt = time.Now()

	if err := database.DB.Save(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update product"})
	}

	// 更新產品稅關聯（先清空再重建）
	database.DB.Where("product_id = ?", product.ID).Delete(&models.ProductTaxRelation{})
	if len(req.ProductTaxIDs) > 0 {
		now := time.Now()
		for _, taxID := range req.ProductTaxIDs {
			if taxID == uuid.Nil {
				continue
			}
			_ = database.DB.Create(&models.ProductTaxRelation{
				ProductID: product.ID,
				TaxID:     taxID,
				CreatedAt: now,
			}).Error
		}
	}

	// 更新產品標籤關聯（先清空再重建）
	database.DB.Where("product_id = ?", product.ID).Delete(&models.ProductLabelRelation{})
	if len(req.LabelIDs) > 0 {
		now := time.Now()
		for _, labelID := range req.LabelIDs {
			if labelID == uuid.Nil {
				continue
			}
			_ = database.DB.Create(&models.ProductLabelRelation{
				ProductID: product.ID,
				LabelID:   labelID,
				CreatedAt: now,
			}).Error
		}
	}

	// 處理產品屬性關聯
	if req.ProductAttributes != nil {
		// 刪除現有的屬性值
		database.DB.Where("product_id = ?", product.ID).Delete(&models.ProductAttributeValue{})

		// 創建新的屬性值
		for _, attr := range *req.ProductAttributes {
			attrValue := models.ProductAttributeValue{
				ProductID:   product.ID,
				AttributeID: attr.AttributeID,
				Value:       attr.Value,
				Status:      "active",
			}
			if err := database.DB.Create(&attrValue).Error; err != nil {
				// 記錄錯誤但不中斷流程
				fmt.Printf("Failed to create product attribute value: %v\n", err)
			}
		}
	}

	// 觸發 Shopee 同步（非阻塞）
	go shopee.Sync.SyncProductToShopee(tenantID, product.ID, "update")
	// 觸發 Amazon 同步（非阻塞）
	go amazon.Sync.SyncProductToAmazon(tenantID, product.ID)
	// 觸發 Lazada 同步（非阻塞）
	go lazada.Sync.SyncProductToLazada(tenantID, product.ID)
	// 觸發 Rakuten 同步（非阻塞）
	go func() {
		syncService, err := rakuten.NewSyncService(tenantID)
		if err != nil {
			log.Printf("[Rakuten] Failed to create sync service for tenant %s: %v", tenantID, err)
			return
		}
		price := int(product.Price)
		if err := syncService.SyncPrice(product.ID, price); err != nil {
			log.Printf("[Rakuten] Price sync failed for product %s: %v", product.ID, err)
		}
	}()

	return c.JSON(product)
}

// DeleteProduct 刪除產品（軟刪除：移到垃圾筒）
func DeleteProduct(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid product ID"})
	}

	// 檢查是否為系統預設資料
	if IsSystemDefault(database.DB, "products", id, tenantID) {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot delete system default data"})
	}

	// 軟刪除：設置 trashed_at 時間
	result := database.DB.Model(&models.Product{}).Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).Update("trashed_at", time.Now())
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete product"})
	}
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Product not found"})
	}

	// 觸發 Shopee 同步（非阻塞）
	go shopee.Sync.SyncProductToShopee(tenantID, id, "delete")
	// 觸發 Amazon 同步（非阻塞）
	go amazon.Sync.SyncProductDeletionToAmazon(tenantID, id)
	// 觸發 Lazada 同步（非阻塞）
	go lazada.Sync.SyncProductDeletionToLazada(tenantID, id)
	// 觸發 Rakuten 同步（非阻塞）- 下架商品
	go func() {
		syncService, err := rakuten.NewSyncService(tenantID)
		if err != nil {
			log.Printf("[Rakuten] Failed to create sync service for tenant %s: %v", tenantID, err)
			return
		}
		mapping, err := syncService.GetProductMapping(id)
		if err != nil {
			return // Product not mapped to Rakuten
		}
		if err := syncService.GetClient().DeactivateItem(mapping.ManageNumber); err != nil {
			log.Printf("[Rakuten] Product deactivation failed for %s: %v", id, err)
		}
	}()

	return c.JSON(fiber.Map{
		"message": "Product moved to trash",
		"info":    "Data will be automatically deleted after 7 days",
	})
}

// ExportProductsToExcel 導出產品到 Excel
func ExportProductsToExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var products []models.Product
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ? OR sku ILIKE ? OR barcode ILIKE ? OR description ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Order("created_at DESC").Find(&products).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch products"})
	}

	f := excelize.NewFile()
	defer f.Close()
	sheetName := "產品列表"
	f.NewSheet(sheetName)
	f.DeleteSheet("Sheet1")

	headers := []string{"編號", "SKU", "條碼", "名稱", "分類", "價格", "成本", "庫存", "單位", "狀態"}
	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheetName, cell, header)
		style, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
		})
		f.SetCellStyle(sheetName, cell, cell, style)
	}

	for i, product := range products {
		row := i + 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), product.Code)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), product.SKU)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), product.Barcode)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), product.Name)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), product.Category)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), product.Price)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), product.Cost)
		f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), product.StockQuantity)
		f.SetCellValue(sheetName, fmt.Sprintf("I%d", row), product.Unit)
		f.SetCellValue(sheetName, fmt.Sprintf("J%d", row), product.Status)
	}

	for i := 0; i < len(headers); i++ {
		f.SetColWidth(sheetName, string(rune('A'+i)), string(rune('A'+i)), 15)
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename=products.xlsx")
	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate Excel file"})
	}
	return nil
}

// ExportProductsToPDF 導出產品到 PDF
func ExportProductsToPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var products []models.Product
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ? OR sku ILIKE ? OR barcode ILIKE ? OR description ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Order("created_at DESC").Find(&products).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch products"})
	}

	headers := []string{"編號", "SKU", "條碼", "名稱", "分類", "價格", "庫存", "狀態"}
	rows := make([][]string, 0, len(products))
	for _, product := range products {
		rows = append(rows, []string{
			product.Code,
			product.SKU,
			product.Barcode,
			product.Name,
			product.Category,
			fmt.Sprintf("%.2f", product.Price),
			fmt.Sprintf("%d", product.StockQuantity),
			product.Status,
		})
	}
	pdfBytes, _ := utils.BuildTablePDFBytes("產品列表", headers, rows)
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", "attachment; filename=products.pdf")
	return c.Send(pdfBytes)
}
