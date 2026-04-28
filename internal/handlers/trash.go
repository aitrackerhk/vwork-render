package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// TrashConfig 垃圾筒配置（用於通用垃圾筒 API）
type TrashConfig struct {
	TableName       string
	Model           interface{}
	SearchFields    []string // 搜索欄位
	SelectFields    string   // 選擇欄位
	PreloadFields   []string // 預加載的關聯
	DisplayName     string   // 顯示名稱（用於錯誤訊息）
	BeforeRestore   func(c *fiber.Ctx, id uuid.UUID, tenantID uuid.UUID) error
	BeforePermanent func(c *fiber.Ctx, id uuid.UUID, tenantID uuid.UUID) error
}

// 垃圾筒資源配置對應表
var trashConfigs = map[string]TrashConfig{
	"orders": {
		TableName:     "orders",
		Model:         &models.Order{},
		SearchFields:  []string{"order_number"},
		PreloadFields: []string{"Customer"},
		DisplayName:   "訂單",
	},
	"quotations": {
		TableName:     "orders",
		Model:         &models.Order{},
		SearchFields:  []string{"order_number"},
		PreloadFields: []string{"Customer"},
		DisplayName:   "報價單",
	},
	"service-orders": {
		TableName:     "service_orders",
		Model:         &models.ServiceOrder{},
		SearchFields:  []string{"order_number"},
		PreloadFields: []string{"Customer"},
		DisplayName:   "服務單",
	},
	"purchase-orders": {
		TableName:     "purchase_orders",
		Model:         &models.PurchaseOrder{},
		SearchFields:  []string{"order_number"},
		PreloadFields: []string{"Supplier"},
		DisplayName:   "採購單",
	},
	"customers": {
		TableName:    "customers",
		Model:        &models.Customer{},
		SearchFields: []string{"name", "code", "email", "phone"},
		DisplayName:  "客戶",
	},
	"products": {
		TableName:    "products",
		Model:        &models.Product{},
		SearchFields: []string{"name", "code"},
		DisplayName:  "產品",
	},
	"services": {
		TableName:    "services",
		Model:        &models.Service{},
		SearchFields: []string{"name", "code"},
		DisplayName:  "服務",
	},
	"suppliers": {
		TableName:    "suppliers",
		Model:        &models.Supplier{},
		SearchFields: []string{"name", "code"},
		DisplayName:  "供應商",
	},
	"users": {
		TableName:    "users",
		Model:        &models.User{},
		SearchFields: []string{"name", "email"},
		DisplayName:  "用戶",
		BeforeRestore: func(c *fiber.Ctx, id uuid.UUID, tenantID uuid.UUID) error {
			// 用戶還原前檢查
			return nil
		},
	},
	"warehouses": {
		TableName:    "warehouses",
		Model:        &models.Warehouse{},
		SearchFields: []string{"name", "code"},
		DisplayName:  "倉庫",
	},
	"stores": {
		TableName:    "stores",
		Model:        &models.Store{},
		SearchFields: []string{"name", "code"},
		DisplayName:  "店舖",
	},
	"projects": {
		TableName:    "projects",
		Model:        &models.Project{},
		SearchFields: []string{"name"},
		DisplayName:  "項目",
	},
	"coupons": {
		TableName:    "coupons",
		Model:        &models.Coupon{},
		SearchFields: []string{"code", "name"},
		DisplayName:  "優惠券",
	},
	"incomes": {
		TableName:    "incomes",
		Model:        &models.Income{},
		SearchFields: []string{"description"},
		DisplayName:  "收入",
	},
	"expenses": {
		TableName:    "expenses",
		Model:        &models.Expense{},
		SearchFields: []string{"description"},
		DisplayName:  "支出",
	},
	"messages": {
		TableName:    "messages",
		Model:        &models.Message{},
		SearchFields: []string{"subject", "content"},
		DisplayName:  "訊息",
	},
	"pages": {
		TableName:    "pages",
		Model:        &models.Page{},
		SearchFields: []string{"title", "slug"},
		DisplayName:  "頁面",
	},
	"blogs": {
		TableName:    "blogs",
		Model:        &models.Blog{},
		SearchFields: []string{"title"},
		DisplayName:  "部落格",
	},
	"vehicles": {
		TableName:    "vehicles",
		Model:        &models.Vehicle{},
		SearchFields: []string{"license_plate", "model"},
		DisplayName:  "車輛",
	},
	"shipments": {
		TableName:    "shipments",
		Model:        &models.Shipment{},
		SearchFields: []string{"shipment_number"},
		DisplayName:  "出貨單",
	},
	"appointments": {
		TableName:    "appointments",
		Model:        &models.Appointment{},
		SearchFields: []string{"notes"},
		DisplayName:  "預約",
	},
	"roles": {
		TableName:    "roles",
		Model:        &models.Role{},
		SearchFields: []string{"name"},
		DisplayName:  "角色",
		BeforeRestore: func(c *fiber.Ctx, id uuid.UUID, tenantID uuid.UUID) error {
			// 檢查是否為 admin 角色
			var role models.Role
			if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&role).Error; err == nil {
				if role.Name == "admin" {
					return fmt.Errorf("cannot modify admin role")
				}
			}
			return nil
		},
	},
	"shifts": {
		TableName:    "shifts",
		Model:        &models.Shift{},
		SearchFields: []string{"name"},
		DisplayName:  "班次",
	},
	"member-levels": {
		TableName:    "member_levels",
		Model:        &models.MemberLevel{},
		SearchFields: []string{"name"},
		DisplayName:  "會員等級",
	},
	"currencies": {
		TableName:    "currencies",
		Model:        &models.Currency{},
		SearchFields: []string{"code", "name"},
		DisplayName:  "貨幣",
	},
	"payment-methods": {
		TableName:    "payment_methods",
		Model:        &models.PaymentMethod{},
		SearchFields: []string{"name"},
		DisplayName:  "付款方式",
	},
	"shipping-methods": {
		TableName:    "shipping_methods",
		Model:        &models.ShippingMethod{},
		SearchFields: []string{"name"},
		DisplayName:  "運送方式",
	},
	"bank-accounts": {
		TableName:    "bank_accounts",
		Model:        &models.BankAccount{},
		SearchFields: []string{"name", "bank_name", "account_number"},
		DisplayName:  "銀行帳戶",
	},
	"logistics-companies": {
		TableName:    "logistics_companies",
		Model:        &models.LogisticsCompany{},
		SearchFields: []string{"name"},
		DisplayName:  "物流公司",
	},
	"order-labels": {
		TableName:    "order_labels",
		Model:        &models.OrderLabel{},
		SearchFields: []string{"name"},
		DisplayName:  "訂單標籤",
	},
	"service-order-labels": {
		TableName:    "service_order_labels",
		Model:        &models.ServiceOrderLabel{},
		SearchFields: []string{"name"},
		DisplayName:  "服務單標籤",
	},
	"purchase-order-labels": {
		TableName:    "purchase_order_labels",
		Model:        &models.PurchaseOrderLabel{},
		SearchFields: []string{"name"},
		DisplayName:  "採購標籤",
	},
	"product-labels": {
		TableName:    "product_labels",
		Model:        &models.ProductLabel{},
		SearchFields: []string{"name"},
		DisplayName:  "產品標籤",
	},
	"customer-labels": {
		TableName:    "customer_labels",
		Model:        &models.CustomerLabel{},
		SearchFields: []string{"name"},
		DisplayName:  "客戶標籤",
	},
	"companies": {
		TableName:    "companies",
		Model:        &models.Company{},
		SearchFields: []string{"name"},
		DisplayName:  "公司",
	},
	"departments": {
		TableName:    "departments",
		Model:        &models.Department{},
		SearchFields: []string{"name"},
		DisplayName:  "部門",
	},
	"product-types": {
		TableName:    "product_types",
		Model:        &models.ProductType{},
		SearchFields: []string{"name"},
		DisplayName:  "產品分類",
	},
	"lead-finder-searches": {
		TableName:    "lead_finder_searches",
		Model:        &models.LeadFinderSearch{},
		SearchFields: []string{"keywords", "region"},
		DisplayName:  "搵客搜尋",
	},
}

// GetTrashItems 獲取垃圾筒中的項目
func GetTrashItems(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	resource := c.Params("resource")
	config, ok := trashConfigs[resource]
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid resource type"})
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit
	search := c.Query("search")

	// 只顯示 7 天內的垃圾資料
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)

	// 構建查詢
	query := database.DB.Table(config.TableName).
		Where("tenant_id = ? AND trashed_at IS NOT NULL AND trashed_at > ?", tenantID, sevenDaysAgo)

	// 特殊處理：報價單只查 quotation 狀態
	if resource == "quotations" {
		query = query.Where("status = ?", "quotation")
	}

	// 搜索
	if search != "" && len(config.SearchFields) > 0 {
		searchCondition := ""
		for i, field := range config.SearchFields {
			if i > 0 {
				searchCondition += " OR "
			}
			searchCondition += fmt.Sprintf("%s ILIKE ?", field)
		}
		args := make([]interface{}, len(config.SearchFields))
		for i := range args {
			args[i] = "%" + search + "%"
		}
		query = query.Where(searchCondition, args...)
	}

	// 計算總數
	var total int64
	query.Count(&total)

	// 獲取資料
	var results []map[string]interface{}
	query.Select("id, tenant_id, trashed_at, created_at, updated_at, " + getSelectFieldsForResource(resource)).
		Order("trashed_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&results)

	return c.JSON(fiber.Map{
		"data":     results,
		"total":    total,
		"page":     page,
		"limit":    limit,
		"trashed":  true,
		"resource": resource,
	})
}

// getSelectFieldsForResource 根據資源類型返回要選擇的欄位
func getSelectFieldsForResource(resource string) string {
	switch resource {
	case "orders", "quotations":
		return "order_number, status, total_amount, customer_id, order_date"
	case "service-orders":
		return "order_number, status, total_amount, customer_id"
	case "purchase-orders":
		return "order_number, status, total_amount, supplier_id"
	case "customers":
		return "name, code, email, phone, status"
	case "products":
		return "name, code, price, status"
	case "services":
		return "name, code, price, status"
	case "suppliers":
		return "name, code, email, phone"
	case "users":
		return "name, email, role_id"
	case "lead-finder-searches":
		return "keywords, region, status, result_count"
	default:
		return "name"
	}
}

// RestoreTrashItem 從垃圾筒還原項目
func RestoreTrashItem(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	resource := c.Params("resource")
	config, ok := trashConfigs[resource]
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid resource type"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	// 執行自定義的前置檢查
	if config.BeforeRestore != nil {
		if err := config.BeforeRestore(c, id, tenantID); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}

	// 清除 trashed_at 時間
	result := database.DB.Table(config.TableName).
		Where("id = ? AND tenant_id = ? AND trashed_at IS NOT NULL", id, tenantID).
		Update("trashed_at", nil)

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to restore %s: %v", config.DisplayName, result.Error)})
	}

	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": fmt.Sprintf("%s not found in trash", config.DisplayName)})
	}

	return c.JSON(fiber.Map{"message": fmt.Sprintf("%s restored successfully", config.DisplayName)})
}

// PermanentDeleteTrashItem 永久刪除垃圾筒中的項目
func PermanentDeleteTrashItem(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	resource := c.Params("resource")
	config, ok := trashConfigs[resource]
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid resource type"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	// 執行自定義的前置檢查
	if config.BeforePermanent != nil {
		if err := config.BeforePermanent(c, id, tenantID); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}

	// 將 trashed_at 設為 8 天前，使其不再顯示在垃圾筒
	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	result := database.DB.Table(config.TableName).
		Where("id = ? AND tenant_id = ? AND trashed_at IS NOT NULL", id, tenantID).
		Update("trashed_at", eightDaysAgo)

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to permanently delete %s: %v", config.DisplayName, result.Error)})
	}

	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": fmt.Sprintf("%s not found in trash", config.DisplayName)})
	}

	return c.JSON(fiber.Map{"message": fmt.Sprintf("%s permanently deleted", config.DisplayName)})
}

// BulkRestoreTrashItems 批量還原垃圾筒項目
func BulkRestoreTrashItems(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	resource := c.Params("resource")
	config, ok := trashConfigs[resource]
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid resource type"})
	}

	var body struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if len(body.IDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No items to restore"})
	}

	// 解析 UUIDs
	ids := make([]uuid.UUID, 0, len(body.IDs))
	for _, idStr := range body.IDs {
		if id, err := uuid.Parse(idStr); err == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No valid IDs provided"})
	}

	// 批量還原
	result := database.DB.Table(config.TableName).
		Where("id IN ? AND tenant_id = ? AND trashed_at IS NOT NULL", ids, tenantID).
		Update("trashed_at", nil)

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to restore items: %v", result.Error)})
	}

	return c.JSON(fiber.Map{
		"message":  fmt.Sprintf("%d items restored successfully", result.RowsAffected),
		"restored": result.RowsAffected,
	})
}

// BulkPermanentDeleteTrashItems 批量永久刪除垃圾筒項目
func BulkPermanentDeleteTrashItems(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	resource := c.Params("resource")
	config, ok := trashConfigs[resource]
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid resource type"})
	}

	var body struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if len(body.IDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No items to delete"})
	}

	// 解析 UUIDs
	ids := make([]uuid.UUID, 0, len(body.IDs))
	for _, idStr := range body.IDs {
		if id, err := uuid.Parse(idStr); err == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No valid IDs provided"})
	}

	// 批量設為過期（8 天前）
	eightDaysAgo := time.Now().AddDate(0, 0, -8)
	result := database.DB.Table(config.TableName).
		Where("id IN ? AND tenant_id = ? AND trashed_at IS NOT NULL", ids, tenantID).
		Update("trashed_at", eightDaysAgo)

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete items: %v", result.Error)})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("%d items permanently deleted", result.RowsAffected),
		"deleted": result.RowsAffected,
	})
}

// GetSupportedTrashResources 獲取支持垃圾筒的資源列表
func GetSupportedTrashResources(c *fiber.Ctx) error {
	resources := make([]map[string]string, 0, len(trashConfigs))
	for key, config := range trashConfigs {
		resources = append(resources, map[string]string{
			"key":         key,
			"displayName": config.DisplayName,
			"tableName":   config.TableName,
		})
	}
	return c.JSON(fiber.Map{"resources": resources})
}
