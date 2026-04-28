package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

func normalizeOrderSourceType(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "erp", "pos", "webstore":
		return s
	default:
		return "erp"
	}
}

// 從 extra_fields 的 shipping_notes / shipping_records 匯總運費（shipping_fee）
func sumOrderShippingFees(extraFields map[string]interface{}) float64 {
	if extraFields == nil {
		return 0
	}
	sum := 0.0
	readFee := func(v interface{}) float64 {
		switch x := v.(type) {
		case float64:
			return x
		case float32:
			return float64(x)
		case int:
			return float64(x)
		case int64:
			return float64(x)
		case json.Number:
			f, _ := x.Float64()
			return f
		case string:
			s := strings.TrimSpace(x)
			if s == "" {
				return 0
			}
			f, _ := strconv.ParseFloat(s, 64)
			return f
		default:
			return 0
		}
	}
	readList := func(key string) {
		raw, ok := extraFields[key]
		if !ok || raw == nil {
			return
		}
		arr, ok := raw.([]interface{})
		if !ok {
			return
		}
		for _, it := range arr {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			sum += readFee(m["shipping_fee"])
		}
	}
	readList("shipping_notes")
	readList("shipping_records")
	return sum
}

// 從 ExtraFields 中取得餐桌 ID
func getDiningTableIDFromExtraFields(extraFields models.JSONB) uuid.UUID {
	if extraFields == nil {
		return uuid.Nil
	}
	fields := map[string]interface{}(extraFields)
	if fields == nil {
		return uuid.Nil
	}
	raw, ok := fields["dining_table_id"]
	if !ok || raw == nil {
		return uuid.Nil
	}
	switch v := raw.(type) {
	case uuid.UUID:
		return v
	case string:
		id, err := uuid.Parse(strings.TrimSpace(v))
		if err == nil {
			return id
		}
	default:
		return uuid.Nil
	}
	return uuid.Nil
}

func normalizeDiningTableExtraFields(tenantID uuid.UUID, extraFields map[string]interface{}) map[string]interface{} {
	if extraFields == nil {
		return extraFields
	}
	raw, ok := extraFields["dining_table_id"]
	if !ok {
		return extraFields
	}
	if raw == nil {
		delete(extraFields, "dining_table_id")
		delete(extraFields, "dining_table_code")
		delete(extraFields, "dining_area_id")
		delete(extraFields, "dining_area_name")
		delete(extraFields, "dining")
		return extraFields
	}
	var tableID uuid.UUID
	switch v := raw.(type) {
	case uuid.UUID:
		tableID = v
	case string:
		vv := strings.TrimSpace(v)
		if vv == "" {
			delete(extraFields, "dining_table_id")
			delete(extraFields, "dining_table_code")
			delete(extraFields, "dining_area_id")
			delete(extraFields, "dining_area_name")
			delete(extraFields, "dining")
			return extraFields
		}
		id, err := uuid.Parse(vv)
		if err == nil {
			tableID = id
		}
	}
	if tableID == uuid.Nil {
		return extraFields
	}

	var table models.DiningTable
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, tableID).Preload("Area").First(&table).Error; err != nil {
		return extraFields
	}

	extraFields["dining_table_id"] = table.ID.String()
	extraFields["dining_table_code"] = table.Code
	if table.AreaID != nil {
		extraFields["dining_area_id"] = table.AreaID.String()
	}
	if table.Area != nil {
		extraFields["dining_area_name"] = table.Area.Name
	}
	extraFields["dining"] = true
	return extraFields
}

func desiredDiningTableStatusByOrderStatus(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "completed", "cancelled":
		return "available"
	default:
		return "occupied"
	}
}

func updateDiningTableStatusByOrder(tenantID uuid.UUID, tableID uuid.UUID, orderStatus string) {
	if tableID == uuid.Nil {
		return
	}
	status := desiredDiningTableStatusByOrderStatus(orderStatus)
	database.DB.Model(&models.DiningTable{}).
		Where("tenant_id = ? AND id = ?", tenantID, tableID).
		Update("status", status)
}

// GetOrders 獲取訂單列表
func GetOrders(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var orders []models.Order
	query := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).Preload("Customer").Preload("OrderItems.Product").Preload("Labels")

	// 默認排除報價單狀態（除非明確指定要顯示）
	excludeQuotation := c.Query("exclude_quotation")
	if excludeQuotation == "true" || (excludeQuotation == "" && c.Query("status") == "") {
		query = query.Where("status != ?", "quotation")
	}

	// 搜索過濾（訂單號 + 客戶名 + 產品名）
	if search := c.Query("search"); search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"(order_number ILIKE ? OR EXISTS (SELECT 1 FROM customers WHERE customers.id = orders.customer_id AND customers.name ILIKE ?) OR EXISTS (SELECT 1 FROM order_items oi LEFT JOIN products p ON oi.product_id = p.id WHERE oi.order_id = orders.id AND oi.trashed_at IS NULL AND (p.name ILIKE ? OR oi.item_name ILIKE ?)))",
			like, like, like, like,
		)
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 客戶過濾
	if customerID := c.Query("customer_id"); customerID != "" {
		if id, err := uuid.Parse(customerID); err == nil {
			query = query.Where("customer_id = ?", id)
		}
	}

	// 標籤過濾（支援多選）
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN order_label_relations ON orders.id = order_label_relations.order_id").
			Where("order_label_relations.label_id IN ?", labelIDs).
			Group("orders.id")
	}

	// 來源過濾
	if sourceType := strings.ToLower(strings.TrimSpace(c.Query("source_type"))); sourceType != "" {
		switch sourceType {
		case "dining", "erp", "pos", "webstore":
			query = query.Where("source_type = ?", sourceType)
		}
	}

	// extraFields 過濾: dining_table_id
	if diningTableID := strings.TrimSpace(c.Query("dining_table_id")); diningTableID != "" {
		query = query.Where("extra_fields->>'dining_table_id' = ?", diningTableID)
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Order{}).Count(&total)

	// 預設按建立時間倒序（其次按訂單日期倒序）顯示
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC, order_date DESC").Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch orders"})
	}

	// 為每張訂單計算不重複的產品名稱列表
	type orderWithProducts struct {
		models.Order
		Products []string `json:"products"`
	}
	results := make([]orderWithProducts, len(orders))
	for i, o := range orders {
		results[i].Order = o
		seen := map[string]bool{}
		for _, item := range o.OrderItems {
			var name string
			if item.Product != nil {
				name = item.Product.Name
			} else if item.ItemName != nil && *item.ItemName != "" {
				name = *item.ItemName
			}
			if name != "" && !seen[name] {
				seen[name] = true
				results[i].Products = append(results[i].Products, name)
			}
		}
		if results[i].Products == nil {
			results[i].Products = []string{}
		}
	}

	return c.JSON(fiber.Map{
		"data":  results,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetQuotations 獲取報價單列表（只顯示 quotation 狀態的訂單）
func GetQuotations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var orders []models.Order
	query := database.DB.Where("tenant_id = ? AND status = ? AND trashed_at IS NULL", tenantID, "quotation").Preload("Customer").Preload("OrderItems.Product").Preload("Labels")

	// 搜索過濾（報價單號 + 客戶名 + 產品名）
	if search := c.Query("search"); search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"(order_number ILIKE ? OR EXISTS (SELECT 1 FROM customers WHERE customers.id = orders.customer_id AND customers.name ILIKE ?) OR EXISTS (SELECT 1 FROM order_items oi LEFT JOIN products p ON oi.product_id = p.id WHERE oi.order_id = orders.id AND oi.trashed_at IS NULL AND (p.name ILIKE ? OR oi.item_name ILIKE ?)))",
			like, like, like, like,
		)
	}

	// 客戶過濾
	if customerID := c.Query("customer_id"); customerID != "" {
		if id, err := uuid.Parse(customerID); err == nil {
			query = query.Where("customer_id = ?", id)
		}
	}

	// 標籤過濾（支援多選）
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN order_label_relations ON orders.id = order_label_relations.order_id").
			Where("order_label_relations.label_id IN ?", labelIDs).
			Group("orders.id")
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Order{}).Count(&total)

	// 預設按建立時間倒序（其次按訂單日期倒序）顯示
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC, order_date DESC").Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch quotations"})
	}

	// 為每張報價單計算不重複的產品名稱列表
	type orderWithProducts struct {
		models.Order
		Products []string `json:"products"`
	}
	results := make([]orderWithProducts, len(orders))
	for i, o := range orders {
		results[i].Order = o
		seen := map[string]bool{}
		for _, item := range o.OrderItems {
			var name string
			if item.Product != nil {
				name = item.Product.Name
			} else if item.ItemName != nil && *item.ItemName != "" {
				name = *item.ItemName
			}
			if name != "" && !seen[name] {
				seen[name] = true
				results[i].Products = append(results[i].Products, name)
			}
		}
		if results[i].Products == nil {
			results[i].Products = []string{}
		}
	}

	return c.JSON(fiber.Map{
		"data":  results,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetOrder 獲取單個訂單
func GetOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order ID"})
	}

	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("Customer").Preload("OrderItems.Product").Preload("Labels").Preload("Salesperson").Preload("Store").First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}

	// 從 ExtraFields 中提取 payment_records 和 shipping_records
	result := make(map[string]interface{})
	result["id"] = order.ID
	result["tenant_id"] = order.TenantID
	result["order_number"] = order.OrderNumber
	result["customer_id"] = order.CustomerID
	result["customer"] = order.Customer
	result["contact_name"] = order.ContactName
	result["contact_email"] = order.ContactEmail
	result["contact_phone"] = order.ContactPhone
	result["contact_address"] = order.ContactAddress
	result["shipping_method_id"] = order.ShippingMethodID
	result["salesperson_id"] = order.SalespersonID
	result["commission_amount"] = order.CommissionAmount
	result["order_date"] = order.OrderDate
	result["status"] = order.Status
	result["total_amount"] = order.TotalAmount
	result["notes"] = order.Notes
	result["referral_code"] = order.ReferralCode
	result["order_items"] = order.OrderItems
	result["labels"] = order.Labels
	result["created_at"] = order.CreatedAt
	result["updated_at"] = order.UpdatedAt
	result["source_type"] = order.SourceType

	// 從 income 表獲取付款記錄（不再從 ExtraFields 讀取）
	var incomes []models.Income
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", order.TenantID, "order", order.ID).Find(&incomes)
	if len(incomes) > 0 {
		paymentRecords := make([]map[string]interface{}, len(incomes))
		for i, inc := range incomes {
			ef := map[string]interface{}{}
			if inc.ExtraFields != nil {
				ef = map[string]interface{}(inc.ExtraFields)
			}
			paymentRecords[i] = map[string]interface{}{
				"income_id":         inc.ID.String(),
				"payment_date":      inc.IncomeDate.Format("2006-01-02"),
				"payment_method":    inc.PaymentMethod,
				"payment_method_id": ef["payment_method_id"],
				"bank_account_id":   inc.BankAccountID,
				"amount":            inc.Amount,
				"invoice_number":    ef["invoice_number"],
				"reference_number":  ef["reference_number"],
				"notes":             inc.Notes,
			}
		}
		result["payment_records"] = paymentRecords
	}

	// 從 ExtraFields 中提取 shipping_records 和 refund_notes
	if order.ExtraFields != nil {
		fields := map[string]interface{}(order.ExtraFields)
		if shippingRecords, exists := fields["shipping_records"]; exists {
			result["shipping_records"] = shippingRecords
		}
		if refundNotes, exists := fields["refund_notes"]; exists {
			result["refund_notes"] = refundNotes
		}
		// 返回完整的 extra_fields，以便前端可以访问所有字段
		result["extra_fields"] = order.ExtraFields
	} else {
		result["extra_fields"] = nil
	}

	return c.JSON(result)
}

// CreateOrder 創建訂單
func CreateOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		OrderNumber      string      `json:"order_number"`
		SourceType       string      `json:"source_type"` // erp / pos / webstore
		CustomerID       *uuid.UUID  `json:"customer_id"`
		ContactName      string      `json:"contact_name"`
		ContactEmail     string      `json:"contact_email"`
		ContactPhone     string      `json:"contact_phone"`
		ContactAddress   string      `json:"contact_address"`
		ShippingMethodID *uuid.UUID  `json:"shipping_method_id"`
		SalespersonID    *uuid.UUID  `json:"salesperson_id"`
		CommissionAmount float64     `json:"commission_amount"`
		OrderDate        string      `json:"order_date"`
		Status           string      `json:"status"`
		Notes            string      `json:"notes"`
		ReferralCode     string      `json:"referral_code"`
		CouponID         *uuid.UUID  `json:"coupon_id"`
		CouponDiscount   float64     `json:"coupon_discount"`
		PointsUsed       int         `json:"points_used"`
		PointsDiscount   float64     `json:"points_discount"`
		LabelIDs         []uuid.UUID `json:"label_ids"`
		OrderItems       []struct {
			ProductID         *uuid.UUID `json:"product_id"`
			Quantity          float64    `json:"quantity"`
			UnitPrice         float64    `json:"unit_price"`
			Notes             string     `json:"notes"`
			ProductAttributes []struct {
				AttributeID string `json:"attribute_id"`
				Value       string `json:"value"`
			} `json:"product_attributes"`
		} `json:"order_items"`
		PaymentRecords []map[string]interface{} `json:"payment_records"`
		ExtraFields    map[string]interface{}   `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := middleware.EnforceTrialOrderLimit(c, tenantID, userID); err != nil {
		return err
	}

	// 如果沒有提供訂單號，自動生成
	if req.OrderNumber == "" {
		orderNumber, err := utils.GenerateOrderNumber(tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to generate order number: " + err.Error()})
		}
		req.OrderNumber = orderNumber
	}

	// 解析日期（使用租戶時區）
	orderDate, err := utils.ParseDateInTenantTimezone(tenantID, req.OrderDate)
	if err != nil {
		orderDate = utils.NowInTenantTimezone(tenantID)
	}

	if req.Status == "" {
		req.Status = "draft"
	}

	// 如果沒有提供referral_code，嘗試從客戶信息中獲取
	if req.ReferralCode == "" && req.CustomerID != nil {
		var customer models.Customer
		if err := database.DB.Where("id = ? AND tenant_id = ?", req.CustomerID, tenantID).First(&customer).Error; err == nil {
			req.ReferralCode = customer.ReferralCode
		}
	}

	now := utils.NowInTenantTimezone(tenantID)

	// 處理 ExtraFields（不再包含 payment_records）
	extraFields := make(map[string]interface{})
	if req.ExtraFields != nil {
		for k, v := range req.ExtraFields {
			// 跳過 payment_records，不再保存到 ExtraFields
			if k != "payment_records" {
				extraFields[k] = v
			}
		}
	}
	inventorySettings := getInventorySettingsForTenant(tenantID)
	applyAutoCompleteInventoryNotes(inventorySettings, extraFields, "shipping_notes")
	// 訂單來源：若未提供，預設為 ERP（CMS 建單）
	if _, ok := extraFields["source_type"]; !ok {
		extraFields["source_type"] = "erp"
	}
	// 處理 shipping_records（如果存在）
	if req.ExtraFields != nil {
		if shippingRecords, exists := req.ExtraFields["shipping_records"]; exists {
			if recordsList, ok := shippingRecords.([]interface{}); ok {
				// 為每個出貨記錄自動生成發貨單號碼（如果還沒有）
				for _, r := range recordsList {
					if record, ok := r.(map[string]interface{}); ok {
						if shippingNum, ok := record["shipping_number"].(string); !ok || shippingNum == "" {
							// 自動生成發貨單號碼
							if shippingNumber, err := utils.GenerateShippingNumber(tenantID); err == nil {
								record["shipping_number"] = shippingNumber
							}
						}
					}
				}
				extraFields["shipping_records"] = recordsList
			}
		}
	}

	// 餐飲餐桌資訊正規化（補齊 table code/area）
	extraFields = normalizeDiningTableExtraFields(tenantID, extraFields)

	order := models.Order{
		TenantID:         tenantID,
		OrderNumber:      req.OrderNumber,
		SourceType:       normalizeOrderSourceType(req.SourceType),
		CustomerID:       req.CustomerID,
		ContactName:      req.ContactName,
		ContactEmail:     req.ContactEmail,
		ContactPhone:     req.ContactPhone,
		ContactAddress:   req.ContactAddress,
		ShippingMethodID: req.ShippingMethodID,
		SalespersonID:    req.SalespersonID,
		CommissionAmount: req.CommissionAmount,
		OrderDate:        orderDate,
		Status:           req.Status,
		Notes:            req.Notes,
		ReferralCode:     req.ReferralCode,
		CouponID:         req.CouponID,
		CouponDiscount:   req.CouponDiscount,
		PointsUsed:       req.PointsUsed,
		PointsDiscount:   req.PointsDiscount,
		CreatedBy:        &userID,
		UpdatedBy:        &userID,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExtraFields:      models.JSONB(extraFields),
	}

	// 計算總金額
	var totalAmount float64
	for _, item := range req.OrderItems {
		itemTotal := item.Quantity * item.UnitPrice
		totalAmount += itemTotal
	}
	shippingFee := sumOrderShippingFees(extraFields)
	order.TotalAmount = totalAmount + shippingFee

	// 計算積分（如果有客戶且積分設置存在）
	var pointsEarned int = 0
	if req.CustomerID != nil {
		var pointSetting models.PointSetting
		if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
			// 檢查是否開啟訂單消費獲得積分
			if pointSetting.EnableOrderEarnPoints && pointSetting.PointsPerDollar > 0 {
				// 計算實際支付金額（總金額 - 折扣）
				actualAmount := totalAmount - req.CouponDiscount - req.PointsDiscount
				if actualAmount > 0 {
					pointsEarned = int(actualAmount * pointSetting.PointsPerDollar)
				}
			}
		}
	}
	order.PointsEarned = pointsEarned

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		log.Printf("Error creating order: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create order: " + err.Error()})
	}

	// 創建訂單明細
	for _, itemReq := range req.OrderItems {
		itemTotal := itemReq.Quantity * itemReq.UnitPrice

		// 處理產品屬性，存儲在 ExtraFields 中
		extraFields := models.JSONB{}
		if len(itemReq.ProductAttributes) > 0 {
			// 將 product_attributes 轉換為可序列化的格式
			attrs := make([]map[string]interface{}, len(itemReq.ProductAttributes))
			for i, attr := range itemReq.ProductAttributes {
				attrs[i] = map[string]interface{}{
					"attribute_id": attr.AttributeID,
					"value":        attr.Value,
				}
			}
			extraFields["product_attributes"] = attrs
		}

		orderItem := models.OrderItem{
			TenantID:    tenantID,
			OrderID:     order.ID,
			ProductID:   itemReq.ProductID,
			Quantity:    itemReq.Quantity,
			UnitPrice:   itemReq.UnitPrice,
			TotalPrice:  itemTotal,
			Notes:       itemReq.Notes,
			ExtraFields: extraFields,
			CreatedAt:   now,
		}

		if err := tx.Create(&orderItem).Error; err != nil {
			tx.Rollback()
			log.Printf("Error creating order item: %v, ProductID: %v, ExtraFields: %+v", err, itemReq.ProductID, extraFields)
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create order item: " + err.Error()})
		}
	}

	// 處理標籤關聯
	if len(req.LabelIDs) > 0 {
		for _, labelID := range req.LabelIDs {
			// 驗證標籤屬於當前租戶
			var label models.OrderLabel
			if err := tx.Where("id = ? AND tenant_id = ?", labelID, tenantID).First(&label).Error; err == nil {
				relation := models.OrderLabelRelation{
					OrderID: order.ID,
					LabelID: labelID,
				}
				tx.Create(&relation)
			}
		}
	}

	// 為每個付款記錄創建 income 記錄（只從 PaymentRecords 讀取，不再從 ExtraFields 讀取）
	if len(req.PaymentRecords) > 0 {
		for _, record := range req.PaymentRecords {
			// 提取付款記錄信息
			amount, _ := record["amount"].(float64)
			if amount > 0 {
				paymentDateStr, _ := record["payment_date"].(string)
				paymentMethod, _ := record["payment_method"].(string)
				paymentMethodID, _ := record["payment_method_id"].(string)
				bankAccountIDStr, _ := record["bank_account_id"].(string)
				notes, _ := record["notes"].(string)
				invoiceNumber, _ := record["invoice_number"].(string)
				referenceNumber, _ := record["reference_number"].(string)

				// 發票號碼（INV-YYYYMMDD-###）：一律透過 reserved_numbers 生成/預留，避免重複與永遠 -001
				if invoiceNumber == "" {
					if n, err := reserveNextNumber(tenantID, "invoice_number", "orders"); err == nil {
						invoiceNumber = n
					}
				} else {
					// 若前端已填，仍嘗試預留（重複則不報錯）
					_, _ = reserveSpecificNumber(tenantID, "invoice_number", invoiceNumber, "orders")
				}

				// 解析日期（使用租戶時區）
				var incomeDate time.Time
				if paymentDateStr != "" {
					if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, paymentDateStr); err == nil {
						incomeDate = parsedDate
					} else {
						incomeDate = order.OrderDate
					}
				} else {
					incomeDate = order.OrderDate
				}

				// 解析銀行賬戶ID
				var bankAccountID *uuid.UUID
				if bankAccountIDStr != "" {
					if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
						bankAccountID = &parsedID
					}
				}

				// 構建標題（只使用 invoice_number）
				description := fmt.Sprintf("訂單 %s 的付款", order.OrderNumber)
				if invoiceNumber != "" {
					description = invoiceNumber
				}

				// 創建 income 記錄
				relatedUserID := &userID
				if order.SalespersonID != nil {
					relatedUserID = order.SalespersonID
				}
				income := models.Income{
					TenantID:      tenantID,
					RelatedUserID: relatedUserID,
					IncomeType:    "order",
					ReferenceID:   &order.ID,
					ReferenceType: "order",
					Category:      "order",
					Description:   description,
					Amount:        amount,
					IncomeDate:    incomeDate,
					PaymentMethod: paymentMethod,
					BankAccountID: bankAccountID,
					Status:        "confirmed",
					Notes:         notes,
					CreatedBy:     &userID,
					UpdatedBy:     &userID,
					CreatedAt:     now,
					UpdatedAt:     now,
					ExtraFields: models.JSONB(map[string]interface{}{
						"payment_method_id": paymentMethodID,
						"invoice_number":    invoiceNumber,
						"reference_number":  referenceNumber,
						"order_id":          order.ID.String(),
					}),
				}

				if err := tx.Create(&income).Error; err != nil {
					// 創建失敗，不中斷訂單創建
				}
			}
		}
	} else {
		// 檢查是否自動生成付款記錄
		auto := getDocumentAutoSettingsForTenant(tenantID)
		shouldAutoGeneratePayment := auto.AutoGenerateOrderPayment && order.TotalAmount > 0

		if shouldAutoGeneratePayment {
			// 如果沒有手動付款記錄且設定開啟，自動生成一個付款記錄
			defPayMethod := defaultReceivingPaymentMethodCode(tenantID)
			defBankAccID := defaultReceivingBankAccountID(tenantID)

			// 自動生成發票號碼（並預留）
			invoiceNumber := ""
			if n, err := reserveNextNumber(tenantID, "invoice_number", "orders"); err == nil {
				invoiceNumber = n
			}

			// 構建標題
			description := fmt.Sprintf("訂單 %s 的付款", order.OrderNumber)
			if invoiceNumber != "" {
				description = invoiceNumber
			}

			// 創建 income 記錄
			relatedUserID := &userID
			if order.SalespersonID != nil {
				relatedUserID = order.SalespersonID
			}
			income := models.Income{
				TenantID:      tenantID,
				RelatedUserID: relatedUserID,
				IncomeType:    "order",
				ReferenceID:   &order.ID,
				ReferenceType: "order",
				Category:      "order",
				Description:   description,
				Amount:        order.TotalAmount,
				IncomeDate:    order.OrderDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defBankAccID,
				Status:        "confirmed",
				Notes:         "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     now,
				UpdatedAt:     now,
				ExtraFields: models.JSONB(map[string]interface{}{
					"invoice_number": invoiceNumber,
					"order_id":       order.ID.String(),
				}),
			}

			if err := tx.Create(&income).Error; err != nil {
				log.Printf("Failed to auto-generate payment record: %v", err)
			}
		}
	}

	// 如果客戶存在且獲得了積分，創建積分記錄並更新客戶總積分
	if req.CustomerID != nil && pointsEarned > 0 {
		// 創建積分記錄
		point := models.Point{
			TenantID:    tenantID,
			CustomerID:  *req.CustomerID,
			Points:      pointsEarned,
			PointsType:  "earned",
			SourceType:  "order",
			SourceID:    &order.ID,
			Description: fmt.Sprintf("訂單 %s 消費獲得積分", order.OrderNumber),
			Status:      "active",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := tx.Create(&point).Error; err != nil {
			// 記錄錯誤但不影響訂單創建
			log.Printf("Failed to create point record: %v", err)
		} else {
			// 更新客戶總積分
			var customer models.Customer
			if err := tx.Where("id = ?", *req.CustomerID).First(&customer).Error; err == nil {
				customer.TotalPoints += pointsEarned
				tx.Save(&customer)
			}
		}
	}

	tx.Commit()

	// 同步 Invoice（在 tx.Commit 後，Income 資料已持久化）
	syncOrderInvoice(database.DB, tenantID, "order", order.ID, &userID)

	// 餐桌狀態更新（餐飲訂單）
	if tableID := getDiningTableIDFromExtraFields(order.ExtraFields); tableID != uuid.Nil {
		updateDiningTableStatusByOrder(tenantID, tableID, order.Status)
	}

	// 處理介紹人積分獎勵（僅在訂單完成時；以被推介客戶的 referral_code 為準）
	if order.CustomerID != nil && req.Status == "completed" {
		refCode := req.ReferralCode
		if refCode == "" {
			refCode = order.ReferralCode
		}
		if refCode != "" {
			// 檢查是否開啟訂單介紹人獎勵積分
			var pointSetting models.PointSetting
			if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
				if pointSetting.EnableOrderReferralBonus {
					processReferralBonus(tenantID, refCode, *order.CustomerID, "訂單", order.ID, order.OrderNumber, pointsEarned)
				}
			} else {
				// 如果沒有設定，預設開啟（向後兼容）
				processReferralBonus(tenantID, refCode, *order.CustomerID, "訂單", order.ID, order.OrderNumber, pointsEarned)
			}
		}
	}

	// 處理會員等級自動升級（檢查所有啟用自動升級的等級，按順序檢查直到不滿足條件）
	// 注意：需要在處理介紹人積分後檢查，因為介紹人可能獲得積分（但這不影響客戶的積分）
	// 但客戶可能因為訂單完成而獲得積分，所以需要重新獲取客戶信息
	if order.CustomerID != nil {
		// 獲取租戶信息
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			// 檢查是否有任何等級啟用了自動升級
			var hasAutoUpgrade bool
			database.DB.Model(&models.MemberLevel{}).
				Where("tenant_id = ? AND auto_upgrade = ? AND status = ?", tenantID, true, "active").
				Select("COUNT(*) > 0").
				Scan(&hasAutoUpgrade)

			if hasAutoUpgrade {
				// 有自動升級等級，檢查客戶是否滿足條件
				checkAndUpgradeMemberLevel(tenantID, *order.CustomerID, order.TotalAmount)
			}
		}
	}

	// 發送訂單確認 email（訂單已確認且有可用 email）
	if req.Status == "confirmed" {
		emailTo := strings.TrimSpace(order.ContactEmail)
		customerName := strings.TrimSpace(order.ContactName)
		customerID := uuid.Nil
		if order.CustomerID != nil {
			customerID = *order.CustomerID
			var customer models.Customer
			if err := database.DB.Where("id = ?", *order.CustomerID).First(&customer).Error; err == nil {
				if emailTo == "" {
					emailTo = strings.TrimSpace(customer.Email)
				}
				if customerName == "" {
					customerName = strings.TrimSpace(customer.Name)
				}
			}
		}
		if emailTo != "" {
			if customerName == "" {
				customerName = "客戶"
			}
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				orderDate := order.OrderDate.Format("2006-01-02")
				if err := email.EnqueueOrderConfirmationEmail(
					tenantID,
					tenant.Subdomain,
					customerID,
					emailTo,
					customerName,
					order.OrderNumber,
					orderDate,
					order.TotalAmount,
					"order",
				); err != nil {
					log.Printf("Failed to enqueue order confirmation email: %v", err)
				}
			}
		}
	}

	// 重新載入關聯數據
	database.DB.Where("id = ?", order.ID).Preload("Customer").Preload("OrderItems.Product").Preload("Labels").First(&order)

	// 若開啟自動生成：保存後直接生成/同步「相關支出」（佣金 / 稅）
	// 報價單不自動生成相關支出
	// 否則：不處理（仍可手動於前端生成）
	if order.Status != "quotation" {
		autoExpenses := getDocumentAutoSettingsForTenant(tenantID)
		if autoExpenses.AutoGenerateOrderCommission || autoExpenses.AutoGenerateOrderTaxes {
			ensureAndSyncOrderRelatedExpenses(tenantID, order.ID, userID, autoExpenses)
		}
	}

	// 通知整個 domain 的所有用戶（檢查通知設置）
	var settings models.NotificationSettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error; err != nil {
		// 如果不存在設定，使用默認值（啟用）
		settings.OrderNotificationsEnabled = true
	}

	if settings.OrderNotificationsEnabled {
		customerName := "散客"
		if order.Customer != nil {
			customerName = order.Customer.Name
		}
		title := fmt.Sprintf("新訂單：%s", order.OrderNumber)
		message := fmt.Sprintf("訂單編號：%s\n客戶：%s\n總金額：$%.2f\n狀態：%s",
			order.OrderNumber, customerName, order.TotalAmount, order.Status)
		link := fmt.Sprintf("/orders?order_id=%s", order.ID.String())
		go utils.CreateNotificationAlertForAllUsers(tenantID, "order_created", title, message, link, &userID)
	}

	// Auto-refresh business goals related to orders
	go RefreshActiveBusinessGoals(tenantID, []string{"order_count", "revenue", "product_sales_qty"})

	return c.Status(201).JSON(order)
}

// processReferralBonus 處理介紹人積分獎勵
func processReferralBonus(tenantID uuid.UUID, referralCode string, referredCustomerID uuid.UUID, sourceType string, sourceID uuid.UUID, sourceNumber string, basePointsEarned int) {
	if referralCode == "" || referredCustomerID == uuid.Nil || sourceID == uuid.Nil {
		return
	}

	// 查找介紹人：優先用 customer.code（前端「我的推薦碼」= customer.code）；兼容舊資料再用 referral_code
	var referrer models.Customer
	if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, referralCode).First(&referrer).Error; err != nil {
		if err := database.DB.Where("tenant_id = ? AND referral_code = ?", tenantID, referralCode).First(&referrer).Error; err != nil {
			return
		}
	}

	// 避免重複發放：同一 source 只給一次
	var dup models.Point
	if err := database.DB.Where("tenant_id = ? AND customer_id = ? AND source_type = ? AND source_id = ? AND points_type = ?",
		tenantID, referrer.ID, "referral", sourceID, "earned").First(&dup).Error; err == nil {
		return
	}

	// 獲取積分設置
	var pointSetting models.PointSetting
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err != nil {
		pointSetting.ReferralBonusMode = "fixed"
		pointSetting.ReferralBonusValue = 0
		pointSetting.ReferralBonusPoints = 0
		pointSetting.ReferralCountPolicy = "all"
	}
	if pointSetting.ReferralBonusMode == "" {
		pointSetting.ReferralBonusMode = "fixed"
	}

	// 計算獎勵積分
	reward := 0
	switch pointSetting.ReferralBonusMode {
	case "percent":
		percent := pointSetting.ReferralBonusValue
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		if basePointsEarned > 0 && percent > 0 {
			reward = int(float64(basePointsEarned) * percent / 100.0)
		}
	default: // fixed
		if pointSetting.ReferralBonusValue > 0 {
			reward = int(pointSetting.ReferralBonusValue + 0.5)
		} else if pointSetting.ReferralBonusPoints > 0 {
			reward = pointSetting.ReferralBonusPoints
		}
	}
	if reward <= 0 {
		return
	}

	// 僅計算第一單：以「被推介客戶」是否已完成過任何單據判斷
	if pointSetting.ReferralCountPolicy == "first_only" {
		var cnt int64
		database.DB.Model(&models.Order{}).
			Where("tenant_id = ? AND customer_id = ? AND status = ?", tenantID, referredCustomerID, "completed").
			Count(&cnt)
		var scnt int64
		database.DB.Table("service_orders").
			Where("tenant_id = ? AND customer_id = ? AND status = ?", tenantID, referredCustomerID, "completed").
			Count(&scnt)
		var pcnt int64
		database.DB.Table("pos_sales").
			Where("tenant_id = ? AND customer_id = ? AND status = ?", tenantID, referredCustomerID, "completed").
			Count(&pcnt)
		// 若完成單據數量 > 1，表示不是首次（此時通常包含當前單據）
		if cnt+scnt+pcnt > 1 {
			return
		}
	}

	// 創建積分記錄
	desc := sourceNumber
	if desc == "" {
		desc = sourceID.String()[:8]
	}
	point := models.Point{
		TenantID:    tenantID,
		CustomerID:  referrer.ID,
		Points:      reward,
		PointsType:  "earned",
		SourceType:  "referral",
		SourceID:    &sourceID,
		Description: fmt.Sprintf("介紹人獎勵（%s：%s）", sourceType, desc),
		Status:      "active",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := database.DB.Create(&point).Error; err != nil {
		return
	}

	referrer.TotalPoints += reward
	database.DB.Save(&referrer)
}

// checkAndUpgradeMemberLevel 檢查並自動升級會員等級（按順序檢查所有等級，直到不滿足條件）
func checkAndUpgradeMemberLevel(tenantID uuid.UUID, customerID uuid.UUID, orderAmount float64) {
	// 獲取租戶信息（用於 email）
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return
	}

	// 重新獲取客戶信息（因為介紹人可能剛獲得積分，客戶的 TotalPoints 可能已更新）
	var customer models.Customer
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, customerID).
		Preload("MemberLevel").
		First(&customer).Error; err != nil {
		return
	}

	// 獲取所有啟用自動升級的會員等級，按順序排序
	var memberLevels []models.MemberLevel
	if err := database.DB.Where("tenant_id = ? AND auto_upgrade = ? AND status = ?", tenantID, true, "active").
		Order("level_order ASC").
		Find(&memberLevels).Error; err != nil {
		return
	}

	if len(memberLevels) == 0 {
		return
	}

	// 計算客戶的總購物金額（已完成訂單的總額，包括當前訂單）
	var totalPurchaseAmount float64
	database.DB.Model(&models.Order{}).
		Where("tenant_id = ? AND customer_id = ? AND status = ?", tenantID, customerID, "completed").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&totalPurchaseAmount)

	// 服務訂單的總額
	var serviceOrderTotal float64
	database.DB.Table("service_orders").
		Where("tenant_id = ? AND customer_id = ? AND status = ?", tenantID, customerID, "completed").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&serviceOrderTotal)
	totalPurchaseAmount += serviceOrderTotal

	// POS 銷售的總額
	var posSaleTotal float64
	database.DB.Table("pos_sales").
		Where("tenant_id = ? AND customer_id = ? AND status = ?", tenantID, customerID, "completed").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&posSaleTotal)
	totalPurchaseAmount += posSaleTotal

	// 如果當前訂單金額不為0，將其加入總額（用於創建訂單時的檢查）
	if orderAmount > 0 {
		totalPurchaseAmount += orderAmount
	}

	// 記錄當前等級名稱（用於 email）
	oldLevelName := "無"
	if customer.MemberLevel != nil {
		oldLevelName = customer.MemberLevel.Name
	}

	// 按順序檢查所有等級，找到符合條件的最高等級
	var targetLevel *models.MemberLevel
	currentLevelOrder := -1
	if customer.MemberLevelID != nil {
		var currentLevel models.MemberLevel
		if err := database.DB.Where("id = ?", customer.MemberLevelID).First(&currentLevel).Error; err == nil {
			currentLevelOrder = currentLevel.LevelOrder
		}
	}

	for i := range memberLevels {
		level := &memberLevels[i]

		// 只檢查比當前等級更高的等級（level_order 更大）
		if currentLevelOrder >= 0 && level.LevelOrder <= currentLevelOrder {
			continue
		}

		// 檢查是否滿足條件
		meetsPoints := level.MinPoints == 0 || customer.TotalPoints >= level.MinPoints
		meetsPurchase := level.MinPurchaseAmount == 0 || totalPurchaseAmount >= level.MinPurchaseAmount

		// 如果同時設定了最低積分和最低購物金額，必須同時滿足
		// 如果只設定了其中一個，只需滿足該條件
		meetsAllConditions := false
		if level.MinPoints > 0 && level.MinPurchaseAmount > 0 {
			meetsAllConditions = meetsPoints && meetsPurchase
		} else if level.MinPoints > 0 {
			meetsAllConditions = meetsPoints
		} else if level.MinPurchaseAmount > 0 {
			meetsAllConditions = meetsPurchase
		} else {
			meetsAllConditions = true
		}

		if meetsAllConditions {
			// 找到符合條件的等級，繼續檢查下一個等級
			targetLevel = level
		} else {
			// 不滿足條件，停止檢查（因為是按順序排列的）
			break
		}
	}

	// 如果找到符合條件的更高等級，進行升級
	if targetLevel != nil {
		// 檢查是否需要升級（避免重複升級到相同等級）
		if customer.MemberLevelID == nil || *customer.MemberLevelID != targetLevel.ID {
			customer.MemberLevelID = &targetLevel.ID
			if err := database.DB.Save(&customer).Error; err != nil {
				log.Printf("Failed to upgrade member level for customer %s: %v", customerID, err)
				return
			}

			// 發送升級 email（如果有 email）
			if customer.Email != "" {
				if err := email.EnqueueMemberLevelUpgradeEmail(
					tenantID,
					tenant.Subdomain,
					customerID,
					customer.Email,
					customer.Name,
					oldLevelName,
					targetLevel.Name,
				); err != nil {
					log.Printf("Failed to enqueue member level upgrade email: %v", err)
				}
			}
		}
	}
}

// UpdateOrder 更新訂單
func UpdateOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order ID"})
	}

	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}

	// 保存原始積分值（用於後續計算差值）
	oldPointsEarned := order.PointsEarned
	oldCustomerID := order.CustomerID

	prevDiningTableID := getDiningTableIDFromExtraFields(order.ExtraFields)

	var req struct {
		CustomerID       *uuid.UUID  `json:"customer_id"`
		SourceType       *string     `json:"source_type"` // erp / pos / webstore
		ContactName      *string     `json:"contact_name"`
		ContactEmail     *string     `json:"contact_email"`
		ContactPhone     *string     `json:"contact_phone"`
		ContactAddress   *string     `json:"contact_address"`
		ShippingMethodID *uuid.UUID  `json:"shipping_method_id"`
		SalespersonID    *uuid.UUID  `json:"salesperson_id"`
		CommissionAmount *float64    `json:"commission_amount"`
		OrderDate        *string     `json:"order_date"`
		Status           *string     `json:"status"`
		Notes            *string     `json:"notes"`
		ReferralCode     *string     `json:"referral_code"`
		LabelIDs         []uuid.UUID `json:"label_ids"`
		TotalAmount      *float64    `json:"total_amount"`
		OrderItems       []struct {
			ProductID         *uuid.UUID `json:"product_id"`
			Quantity          float64    `json:"quantity"`
			UnitPrice         float64    `json:"unit_price"`
			ProductAttributes []struct {
				AttributeID string `json:"attribute_id"`
				Value       string `json:"value"`
			} `json:"product_attributes"`
			Notes string `json:"notes"`
		} `json:"order_items"`
		PaymentRecords  []map[string]interface{} `json:"payment_records"`
		ShippingRecords []map[string]interface{} `json:"shipping_records"`
		ExtraFields     *map[string]interface{}  `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Error parsing body in UpdateOrder: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.CustomerID != nil {
		order.CustomerID = req.CustomerID
	}
	if req.SourceType != nil {
		order.SourceType = normalizeOrderSourceType(*req.SourceType)
	}
	if req.ContactName != nil {
		order.ContactName = *req.ContactName
	}
	if req.ContactEmail != nil {
		order.ContactEmail = *req.ContactEmail
	}
	if req.ContactPhone != nil {
		order.ContactPhone = *req.ContactPhone
	}
	if req.ContactAddress != nil {
		order.ContactAddress = *req.ContactAddress
	}
	if req.ShippingMethodID != nil {
		order.ShippingMethodID = req.ShippingMethodID
	}
	if req.SalespersonID != nil {
		order.SalespersonID = req.SalespersonID
	}
	if req.CommissionAmount != nil {
		order.CommissionAmount = *req.CommissionAmount
	}
	if req.OrderDate != nil {
		if date, err := utils.ParseDateInTenantTimezone(tenantID, *req.OrderDate); err == nil {
			order.OrderDate = date
		}
	}
	oldStatus := order.Status
	if req.Status != nil {
		order.Status = *req.Status
	}
	if req.ReferralCode != nil {
		order.ReferralCode = *req.ReferralCode
	}
	if req.Notes != nil {
		order.Notes = *req.Notes
	}

	// 處理 ExtraFields（不再包含 payment_records）
	if req.ExtraFields != nil {
		fields := *req.ExtraFields
		// 移除 payment_records（如果存在），不再保存到 ExtraFields
		delete(fields, "payment_records")
		fields = normalizeDiningTableExtraFields(tenantID, fields)
		order.ExtraFields = models.JSONB(fields)
	}

	// 更新訂單明細
	if req.OrderItems != nil {
		// 刪除現有明細
		database.DB.Where("order_id = ?", order.ID).Delete(&models.OrderItem{})

		// 計算總金額
		var totalAmount float64
		for _, itemReq := range req.OrderItems {
			itemTotal := itemReq.Quantity * itemReq.UnitPrice
			totalAmount += itemTotal

			// 處理 product_attributes，存儲到 ExtraFields
			extraFields := make(map[string]interface{})
			if len(itemReq.ProductAttributes) > 0 {
				// 將 product_attributes 轉換為可序列化的格式
				attrs := make([]map[string]interface{}, len(itemReq.ProductAttributes))
				for i, attr := range itemReq.ProductAttributes {
					attrs[i] = map[string]interface{}{
						"attribute_id": attr.AttributeID,
						"value":        attr.Value,
					}
				}
				extraFields["product_attributes"] = attrs
			}

			// 創建新明細
			orderItem := models.OrderItem{
				TenantID:    tenantID,
				OrderID:     order.ID,
				ProductID:   itemReq.ProductID,
				Quantity:    itemReq.Quantity,
				UnitPrice:   itemReq.UnitPrice,
				TotalPrice:  itemTotal,
				Notes:       itemReq.Notes,
				ExtraFields: models.JSONB(extraFields),
				CreatedAt:   time.Now(),
			}
			if err := database.DB.Create(&orderItem).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create order item"})
			}
		}
		shippingFee := sumOrderShippingFees(map[string]interface{}(order.ExtraFields))
		order.TotalAmount = totalAmount + shippingFee
	} else if req.TotalAmount != nil {
		order.TotalAmount = *req.TotalAmount
	}

	// 重新計算積分（如果有客戶且積分設置存在）
	var newPointsEarned int = 0
	var pointSetting models.PointSetting
	hasPointSetting := false
	if order.CustomerID != nil {
		if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
			hasPointSetting = true
			// 檢查是否開啟訂單消費獲得積分
			if pointSetting.EnableOrderEarnPoints && pointSetting.PointsPerDollar > 0 {
				// 計算實際支付金額（總金額 - 折扣）
				actualAmount := order.TotalAmount - order.CouponDiscount - order.PointsDiscount
				if actualAmount > 0 {
					newPointsEarned = int(actualAmount * pointSetting.PointsPerDollar)
				}
			}
		}
	}
	order.PointsEarned = newPointsEarned

	order.UpdatedBy = &userID
	order.UpdatedAt = time.Now()

	if err := database.DB.Save(&order).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update order"})
	}

	// ============================================
	// 編輯訂單時積分調整：如果積分增加，補發差值到客戶積分
	// ============================================
	if hasPointSetting && pointSetting.EnablePointsAdjustmentOnEdit && order.CustomerID != nil {
		pointsDiff := newPointsEarned - oldPointsEarned
		// 只處理積分增加的情況（差值為正）
		if pointsDiff > 0 {
			// 檢查客戶是否變更
			sameCustomer := oldCustomerID != nil && *oldCustomerID == *order.CustomerID
			if sameCustomer {
				// 創建積分記錄
				now := time.Now()
				point := models.Point{
					TenantID:    tenantID,
					CustomerID:  *order.CustomerID,
					Points:      pointsDiff,
					PointsType:  "adjustment",
					SourceType:  "order_edit",
					SourceID:    &order.ID,
					Description: fmt.Sprintf("編輯訂單 %s 補發積分差值 (+%d)", order.OrderNumber, pointsDiff),
					Status:      "active",
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				if err := database.DB.Create(&point).Error; err != nil {
					log.Printf("Failed to create point adjustment record for order edit: %v", err)
				} else {
					// 更新客戶總積分
					var customer models.Customer
					if err := database.DB.Where("id = ?", *order.CustomerID).First(&customer).Error; err == nil {
						customer.TotalPoints += pointsDiff
						database.DB.Save(&customer)
						log.Printf("Order edit points adjustment: order=%s, customer=%s, diff=+%d", order.OrderNumber, customer.Name, pointsDiff)
					}
				}
			}
		}
	}

	// 餐桌狀態更新（餐飲訂單）
	newDiningTableID := getDiningTableIDFromExtraFields(order.ExtraFields)
	if prevDiningTableID != uuid.Nil && prevDiningTableID != newDiningTableID {
		updateDiningTableStatusByOrder(tenantID, prevDiningTableID, "completed")
	}
	if newDiningTableID != uuid.Nil {
		updateDiningTableStatusByOrder(tenantID, newDiningTableID, order.Status)
	}

	// 同步已生成的「相關支出」（佣金 / 稅）金額（best-effort）
	// - 報價單不自動生成相關支出
	// - 若開啟自動生成：保存後直接生成/同步「相關支出」（佣金 / 稅）
	// - 否則：只同步已存在支出金額（不自動生成）
	if order.Status != "quotation" {
		auto := getDocumentAutoSettingsForTenant(tenantID)
		if auto.AutoGenerateOrderCommission || auto.AutoGenerateOrderTaxes {
			// IMPORTANT: 同步執行，確保保存完成時「相關支出」已生成/同步，前端才會在保存後跳回列表
			ensureAndSyncOrderRelatedExpenses(tenantID, order.ID, userID, auto)
		} else {
			// 同步執行（保持一致，避免保存後仍在背景更新）
			syncOrderRelatedExpenses(tenantID, order.ID)
		}
	}

	// 處理付款記錄（只保存到 income 表，不再保存到 ExtraFields）
	// 首先獲取所有現有的 income 記錄
	var existingIncomes []models.Income
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "order", order.ID).Find(&existingIncomes)
	existingIncomeIDs := make(map[uuid.UUID]bool)
	for _, inc := range existingIncomes {
		existingIncomeIDs[inc.ID] = true
	}

	// 處理新的付款記錄（即使為空也要處理，以便刪除舊記錄）
	if len(req.PaymentRecords) > 0 {
		// 記錄哪些 income_id 在新的付款記錄中
		keptIncomeIDs := make(map[uuid.UUID]bool)

		// 為新的付款記錄創建或更新 income 記錄
		for _, record := range req.PaymentRecords {
			if record == nil {
				continue
			}

			// 處理金額（可能是 float64 或字符串）
			var amount float64
			amountRaw, exists := record["amount"]
			if !exists {
				continue
			}

			// 處理金額（支持多種類型，包括 json.Number）
			switch v := amountRaw.(type) {
			case float64:
				amount = v
			case float32:
				amount = float64(v)
			case int:
				amount = float64(v)
			case int32:
				amount = float64(v)
			case int64:
				amount = float64(v)
			case string:
				if parsed, err := strconv.ParseFloat(v, 64); err == nil {
					amount = parsed
				}
			default:
				// 嘗試轉換為字符串再解析（處理 json.Number 等類型）
				if strVal := fmt.Sprintf("%v", v); strVal != "" {
					if parsed, err := strconv.ParseFloat(strVal, 64); err == nil {
						amount = parsed
					}
				}
			}

			// 如果金额为0或未正确解析，跳过
			if amount == 0 {
				continue
			}

			if amount > 0 {
				// 處理 income_id（可能是 null, 空字符串, 或 UUID 字符串）
				var incomeIDStr string
				if idVal, exists := record["income_id"]; exists {
					if idVal != nil {
						if idStr, ok := idVal.(string); ok {
							incomeIDStr = idStr
						}
					}
				} else if idVal, exists := record["id"]; exists {
					// 兼容舊的 id 字段
					if idVal != nil {
						if idStr, ok := idVal.(string); ok {
							incomeIDStr = idStr
						}
					}
				}

				// 如果已經有 income_id，更新現有的 income 記錄
				if incomeIDStr != "" {
					if incomeID, err := uuid.Parse(incomeIDStr); err == nil {
						keptIncomeIDs[incomeID] = true
						var existingIncome models.Income
						if err := database.DB.Where("id = ? AND tenant_id = ?", incomeID, tenantID).First(&existingIncome).Error; err == nil {
							// 更新現有的 income 記錄
							paymentDateStr, _ := record["payment_date"].(string)
							paymentMethod, _ := record["payment_method"].(string)
							paymentMethodID, _ := record["payment_method_id"].(string)
							bankAccountIDStr, _ := record["bank_account_id"].(string)
							notes, _ := record["notes"].(string)
							invoiceNumber, _ := record["invoice_number"].(string)
							referenceNumber, _ := record["reference_number"].(string)

							var incomeDate time.Time
							if paymentDateStr != "" {
								if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, paymentDateStr); err == nil {
									incomeDate = parsedDate
								} else {
									incomeDate = order.OrderDate
								}
							} else {
								incomeDate = order.OrderDate
							}

							var bankAccountID *uuid.UUID
							if bankAccountIDStr != "" {
								if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
									bankAccountID = &parsedID
								}
							}

							// 構建 ExtraFields
							incomeExtraFields := make(map[string]interface{})
							if existingIncome.ExtraFields != nil {
								incomeExtraFields = map[string]interface{}(existingIncome.ExtraFields)
							}
							if paymentMethodID != "" {
								incomeExtraFields["payment_method_id"] = paymentMethodID
							}
							if invoiceNumber != "" {
								incomeExtraFields["invoice_number"] = invoiceNumber
							}
							if referenceNumber != "" {
								incomeExtraFields["reference_number"] = referenceNumber
							}
							incomeExtraFields["order_id"] = order.ID.String()

							existingIncome.Amount = amount
							existingIncome.IncomeDate = incomeDate
							existingIncome.PaymentMethod = paymentMethod
							existingIncome.BankAccountID = bankAccountID
							existingIncome.Notes = notes
							existingIncome.ExtraFields = models.JSONB(incomeExtraFields)
							existingIncome.UpdatedBy = &userID
							existingIncome.UpdatedAt = time.Now()

							if err := database.DB.Save(&existingIncome).Error; err != nil {
								// 更新失敗，繼續處理下一個
							}
						}
					}
				} else {
					// 如果沒有 income_id，創建新的 income 記錄
					paymentDateStr, _ := record["payment_date"].(string)
					paymentMethod, _ := record["payment_method"].(string)
					paymentMethodID, _ := record["payment_method_id"].(string)
					bankAccountIDStr, _ := record["bank_account_id"].(string)
					notes, _ := record["notes"].(string)
					invoiceNumber, _ := record["invoice_number"].(string)
					referenceNumber, _ := record["reference_number"].(string)

					var incomeDate time.Time
					if paymentDateStr != "" {
						if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, paymentDateStr); err == nil {
							incomeDate = parsedDate
						} else {
							incomeDate = order.OrderDate
						}
					} else {
						incomeDate = order.OrderDate
					}

					var bankAccountID *uuid.UUID
					if bankAccountIDStr != "" {
						if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
							bankAccountID = &parsedID
						}
					}

					// 構建標題（使用 invoice_number 和 order_number）
					description := fmt.Sprintf("訂單 %s 的付款", order.OrderNumber)
					if invoiceNumber != "" {
						description = invoiceNumber
					}

					// 創建 income 記錄
					relatedUserID := &userID
					if order.SalespersonID != nil {
						relatedUserID = order.SalespersonID
					}
					income := models.Income{
						TenantID:      tenantID,
						RelatedUserID: relatedUserID,
						IncomeType:    "order",
						ReferenceID:   &order.ID,
						ReferenceType: "order",
						Category:      "order",
						Description:   description,
						Amount:        amount,
						IncomeDate:    incomeDate,
						PaymentMethod: paymentMethod,
						BankAccountID: bankAccountID,
						Status:        "confirmed",
						Notes:         notes,
						CreatedBy:     &userID,
						UpdatedBy:     &userID,
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
						ExtraFields: models.JSONB(map[string]interface{}{
							"payment_method_id": paymentMethodID,
							"invoice_number":    invoiceNumber,
							"reference_number":  referenceNumber,
							"order_id":          order.ID.String(),
						}),
					}

					if err := database.DB.Create(&income).Error; err != nil {
						// 創建失敗，繼續處理下一個
					} else {
						keptIncomeIDs[income.ID] = true
					}
				}
			}
		}

		// 刪除不在新付款記錄中的 income 記錄
		for incomeID := range existingIncomeIDs {
			if !keptIncomeIDs[incomeID] {
				database.DB.Where("id = ? AND tenant_id = ?", incomeID, tenantID).Delete(&models.Income{})
			}
		}
	} else {
		// 檢查是否自動生成付款記錄
		auto := getDocumentAutoSettingsForTenant(tenantID)
		shouldAutoGeneratePayment := auto.AutoGenerateOrderPayment && order.TotalAmount > 0

		if shouldAutoGeneratePayment {
			// 如果沒有手動付款記錄且設定開啟，自動生成一個付款記錄
			defPayMethod := defaultReceivingPaymentMethodCode(tenantID)
			defBankAccID := defaultReceivingBankAccountID(tenantID)

			// 自動生成發票號碼（並預留）
			invoiceNumber := ""
			if n, err := reserveNextNumber(tenantID, "invoice_number", "orders"); err == nil {
				invoiceNumber = n
			}

			// 構建標題
			description := fmt.Sprintf("訂單 %s 的付款", order.OrderNumber)
			if invoiceNumber != "" {
				description = invoiceNumber
			}

			// 創建 income 記錄
			relatedUserID := &userID
			if order.SalespersonID != nil {
				relatedUserID = order.SalespersonID
			}
			income := models.Income{
				TenantID:      tenantID,
				RelatedUserID: relatedUserID,
				IncomeType:    "order",
				ReferenceID:   &order.ID,
				ReferenceType: "order",
				Category:      "order",
				Description:   description,
				Amount:        order.TotalAmount,
				IncomeDate:    order.OrderDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defBankAccID,
				Status:        "confirmed",
				Notes:         "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				ExtraFields: models.JSONB(map[string]interface{}{
					"invoice_number": invoiceNumber,
					"order_id":       order.ID.String(),
				}),
			}

			if err := database.DB.Create(&income).Error; err != nil {
				log.Printf("Failed to auto-generate payment record: %v", err)
			}
		} else {
			// 如果沒有付款記錄且不自動生成，刪除所有相關的 income 記錄
			database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "order", order.ID).Delete(&models.Income{})
		}
	}

	// 同步 Invoice（付款記錄已更新完畢）
	syncOrderInvoice(database.DB, tenantID, "order", order.ID, &userID)

	// 處理標籤關聯
	if req.LabelIDs != nil {
		// 刪除現有標籤關聯
		database.DB.Where("order_id = ?", order.ID).Delete(&models.OrderLabelRelation{})

		// 添加新的標籤關聯
		for _, labelID := range req.LabelIDs {
			// 驗證標籤屬於當前租戶
			var label models.OrderLabel
			if err := database.DB.Where("id = ? AND tenant_id = ?", labelID, tenantID).First(&label).Error; err == nil {
				relation := models.OrderLabelRelation{
					OrderID: order.ID,
					LabelID: labelID,
				}
				database.DB.Create(&relation)
			}
		}
	}

	// 處理發貨單並更新庫存（從 shipping_notes）
	// 注意：更新時需要先恢復舊的發貨單庫存，然後應用新的發貨單庫存
	// 為了簡化，這裡只處理新的發貨單（實際應用中可能需要更複雜的邏輯）
	// 跳過外賣訂單 - 外賣平台訂單不使用內置庫存和物流系統
	if req.ExtraFields != nil && order.SourceType != "delivery" {
		inventorySettings := getInventorySettingsForTenant(tenantID)
		applyAutoCompleteInventoryNotes(inventorySettings, *req.ExtraFields, "shipping_notes")
		fields := *req.ExtraFields
		if shippingNotes, exists := fields["shipping_notes"]; exists {
			if notesList, ok := shippingNotes.([]interface{}); ok {
				for _, noteInterface := range notesList {
					if note, ok := noteInterface.(map[string]interface{}); ok {
						warehouseIDStr, _ := note["warehouse_id"].(string)
						if warehouseIDStr != "" {
							warehouseID, err := uuid.Parse(warehouseIDStr)
							if err == nil {
								if items, exists := note["items"].([]interface{}); exists {
									for _, itemInterface := range items {
										if item, ok := itemInterface.(map[string]interface{}); ok {
											productIDStr, _ := item["product_id"].(string)
											quantity, _ := item["quantity"].(float64)

											if productIDStr != "" && quantity > 0 {
												productID, err := uuid.Parse(productIDStr)
												if err == nil {
													// 出貨：減少庫存
													if err := utils.UpdateWarehouseStock(tenantID, productID, warehouseID, int(quantity), "decrease"); err != nil {
														log.Printf("Failed to update warehouse stock for shipping: %v", err)
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// ============================================
	// 退款單（refund_notes）
	// - 退款金額 -> expenses（category=refund）
	// - 取回產品 -> receiving_notes（直接入庫，不創建採購單）
	// - 取回傭金 -> incomes（reference_type=order_refund_commission）
	// 以 refund_notes 內的 *_id 做冪等，避免重複生成
	// 跳過外賣訂單 - 外賣平台訂單不使用內置庫存和物流系統
	// ============================================
	if req.ExtraFields != nil && order.SourceType != "delivery" {
		fields := *req.ExtraFields
		if refundNotes, exists := fields["refund_notes"]; exists {
			notesList, ok := refundNotes.([]interface{})
			if ok && len(notesList) > 0 {
				// 建立訂單產品數量 map（用於限制：退的總量不能超過）
				orderQtyByProduct := map[string]float64{}
				for _, it := range req.OrderItems {
					if it.ProductID == nil || *it.ProductID == uuid.Nil {
						continue
					}
					orderQtyByProduct[it.ProductID.String()] += it.Quantity
				}

				// 聚合所有退款單的產品退款數量
				refundQtyByProduct := map[string]float64{}
				for _, n := range notesList {
					note, ok := n.(map[string]interface{})
					if !ok {
						continue
					}
					itemsRaw, _ := note["items"].([]interface{})
					for _, ir := range itemsRaw {
						im, ok := ir.(map[string]interface{})
						if !ok {
							continue
						}
						pid, _ := im["product_id"].(string)
						if pid == "" {
							continue
						}
						var qty float64
						switch v := im["quantity"].(type) {
						case float64:
							qty = v
						case float32:
							qty = float64(v)
						case int:
							qty = float64(v)
						case int64:
							qty = float64(v)
						case string:
							if parsed, err := strconv.ParseFloat(v, 64); err == nil {
								qty = parsed
							}
						default:
							if s := fmt.Sprintf("%v", v); s != "" {
								if parsed, err := strconv.ParseFloat(s, 64); err == nil {
									qty = parsed
								}
							}
						}
						if qty < 0 {
							qty = 0
						}
						refundQtyByProduct[pid] += qty
					}
				}

				for pid, rq := range refundQtyByProduct {
					if oq, ok := orderQtyByProduct[pid]; ok {
						if rq > oq+1e-9 {
							return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("退款數量超過訂單數量：product_id=%s (退款 %.2f > 訂單 %.2f)", pid, rq, oq)})
						}
					} else if rq > 0 {
						return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("退款產品不在訂單中：product_id=%s", pid)})
					}
				}

				// 取客戶名（作為 vendor）
				vendor := ""
				if order.CustomerID != nil {
					var customer models.Customer
					if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *order.CustomerID).First(&customer).Error; err == nil {
						vendor = customer.Name
					}
				}
				if vendor == "" {
					vendor = "客戶"
				}

				// 找到系統預設倉庫（取回產品用）
				var defaultWarehouse models.Warehouse
				defaultWarehouseID := ""
				if err := database.DB.Where("tenant_id = ? AND is_default = ?", tenantID, true).First(&defaultWarehouse).Error; err == nil {
					defaultWarehouseID = defaultWarehouse.ID.String()
				} else {
					var anyWarehouse models.Warehouse
					if err2 := database.DB.Where("tenant_id = ?", tenantID).Order("created_at ASC").First(&anyWarehouse).Error; err2 == nil {
						defaultWarehouseID = anyWarehouse.ID.String()
					}
				}

				// 查佣金支出（用於計算取回傭金）
				var commissionExpense models.Expense
				commissionExpenseAmount := 0.0
				if err := database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ? AND category = ?", tenantID, "order", order.ID, "commission").
					Order("created_at DESC").First(&commissionExpense).Error; err == nil {
					commissionExpenseAmount = commissionExpense.Amount
				} else if order.CommissionAmount > 0 {
					commissionExpenseAmount = order.CommissionAmount
				}

				// 逐張退款單處理：生成 Expense / PurchaseOrder / Income（冪等）
				// 注意：無論是否生成 expense/income/purchase_order，都要保存 refund_notes 到 extra_fields
				for noteIndex, n := range notesList {
					note, ok := n.(map[string]interface{})
					if !ok {
						continue
					}

					refundNumber, _ := note["refund_number"].(string)
					refundDateStr, _ := note["refund_date"].(string)
					if refundDateStr == "" {
						refundDateStr = order.OrderDate.Format("2006-01-02")
					}
					refundDate, err := utils.ParseDateInTenantTimezone(tenantID, refundDateStr)
					if err != nil {
						refundDate = utils.NowInTenantTimezone(tenantID)
					}

					// flags
					returnProducts := false
					if b, ok := note["return_products"].(bool); ok {
						returnProducts = b
					} else if s, ok := note["return_products"].(string); ok && (s == "true" || s == "1") {
						returnProducts = true
					}
					returnCommission := false
					if b, ok := note["return_commission"].(bool); ok {
						returnCommission = b
					} else if s, ok := note["return_commission"].(string); ok && (s == "true" || s == "1") {
						returnCommission = true
					}

					// items total
					itemsTotal := 0.0
					itemsRaw, _ := note["items"].([]interface{})
					for _, ir := range itemsRaw {
						im, ok := ir.(map[string]interface{})
						if !ok {
							continue
						}
						var qty float64
						switch v := im["quantity"].(type) {
						case float64:
							qty = v
						case float32:
							qty = float64(v)
						case int:
							qty = float64(v)
						case int64:
							qty = float64(v)
						case string:
							if parsed, err := strconv.ParseFloat(v, 64); err == nil {
								qty = parsed
							}
						default:
							if s := fmt.Sprintf("%v", v); s != "" {
								if parsed, err := strconv.ParseFloat(s, 64); err == nil {
									qty = parsed
								}
							}
						}
						if qty < 0 {
							qty = 0
						}

						var unit float64
						switch v := im["unit_price"].(type) {
						case float64:
							unit = v
						case float32:
							unit = float64(v)
						case int:
							unit = float64(v)
						case int64:
							unit = float64(v)
						case string:
							if parsed, err := strconv.ParseFloat(v, 64); err == nil {
								unit = parsed
							}
						default:
							if s := fmt.Sprintf("%v", v); s != "" {
								if parsed, err := strconv.ParseFloat(s, 64); err == nil {
									unit = parsed
								}
							}
						}
						if unit < 0 {
							unit = 0
						}
						itemsTotal += qty * unit
					}

					extraAmount := 0.0
					switch v := note["extra_amount"].(type) {
					case float64:
						extraAmount = v
					case float32:
						extraAmount = float64(v)
					case int:
						extraAmount = float64(v)
					case int64:
						extraAmount = float64(v)
					case string:
						if parsed, err := strconv.ParseFloat(v, 64); err == nil {
							extraAmount = parsed
						}
					default:
						if s := fmt.Sprintf("%v", v); s != "" {
							if parsed, err := strconv.ParseFloat(s, 64); err == nil {
								extraAmount = parsed
							}
						}
					}
					if extraAmount < 0 {
						extraAmount = 0
					}

					totalRefund := itemsTotal + extraAmount
					if totalRefund < 0 {
						totalRefund = 0
					}

					// 1) Expense（退款金額 -> 支出）
					expenseIDStr, _ := note["expense_id"].(string)
					if totalRefund > 0 && expenseIDStr == "" {
						// 退款支出：使用「系統預設支出付款方法 / 付款賬戶」
						paymentMethod := "cash"
						var pm models.PaymentMethod
						if err := database.DB.Where("tenant_id = ? AND status = ? AND is_default_expense = ?", tenantID, "active", true).First(&pm).Error; err == nil && pm.Code != "" {
							paymentMethod = pm.Code
						} else if err := database.DB.Where("tenant_id = ? AND status = ? AND is_default = ?", tenantID, "active", true).First(&pm).Error; err == nil && pm.Code != "" {
							// fallback：舊版只有客戶付款預設
							paymentMethod = pm.Code
						}
						var bankAccountID *uuid.UUID = nil
						{
							var ba models.BankAccount
							if err := database.DB.Where("tenant_id = ? AND status = ? AND is_default_payment = ?", tenantID, "active", true).First(&ba).Error; err == nil && ba.ID != uuid.Nil {
								id := ba.ID
								bankAccountID = &id
							}
						}

						// 退款支出描述：只顯示退款單號（REF-*），列表頁再用 (ORD-*) 顯示關聯目標
						description := refundNumber
						exp := models.Expense{
							TenantID:      tenantID,
							RelatedUserID: order.SalespersonID, // 優先關聯銷售員
							ExpenseType:   "refund",
							ReferenceID:   &order.ID,
							ReferenceType: "order",
							Category:      "refund",
							Description:   description,
							Amount:        totalRefund,
							ExpenseDate:   refundDate,
							PaymentMethod: paymentMethod,
							BankAccountID: bankAccountID,
							Vendor:        vendor,
							Status:        "confirmed",
							Notes:         fmt.Sprintf("refund_note_index=%d", noteIndex),
							CreatedBy:     &userID,
							UpdatedBy:     &userID,
							CreatedAt:     time.Now(),
							UpdatedAt:     time.Now(),
							ExtraFields: models.JSONB(map[string]interface{}{
								"refund_number": refundNumber,
								"order_id":      order.ID.String(),
								"note_index":    noteIndex,
							}),
						}
						if exp.RelatedUserID == nil {
							exp.RelatedUserID = &userID
						}
						if err := database.DB.Create(&exp).Error; err == nil {
							note["expense_id"] = exp.ID.String()
						}
					}

					// 2) 入貨記錄（取回產品 -> 直接入庫，不創建採購單）
					// 如果運送方式「需要送貨」，需檢查發貨單中是否有對應的產品
					if returnProducts {
						// 先檢查運送方式是否需要送貨
						requiresShipping := false
						if order.ShippingMethodID != nil {
							var shippingMethod models.ShippingMethod
							if err := database.DB.Where("id = ?", *order.ShippingMethodID).First(&shippingMethod).Error; err == nil {
								requiresShipping = shippingMethod.RequiresShipping
							}
						}

						// 如果需要送貨，檢查發貨單
						if requiresShipping {
							// 獲取訂單的發貨單
							shippedQtyByProduct := map[string]float64{}
							if shippingNotesRaw, exists := fields["shipping_notes"]; exists {
								if snList, ok := shippingNotesRaw.([]interface{}); ok {
									for _, sn := range snList {
										snMap, ok := sn.(map[string]interface{})
										if !ok {
											continue
										}
										// 只計算已完成的發貨單
										snStatus, _ := snMap["status"].(string)
										if snStatus != "completed" && snStatus != "processing" {
											continue
										}
										snItems, _ := snMap["items"].([]interface{})
										for _, si := range snItems {
											siMap, ok := si.(map[string]interface{})
											if !ok {
												continue
											}
											pid, _ := siMap["product_id"].(string)
											if pid == "" {
												continue
											}
											var qty float64
											switch v := siMap["quantity"].(type) {
											case float64:
												qty = v
											case float32:
												qty = float64(v)
											case int:
												qty = float64(v)
											case int64:
												qty = float64(v)
											case string:
												if parsed, err := strconv.ParseFloat(v, 64); err == nil {
													qty = parsed
												}
											}
											shippedQtyByProduct[pid] += qty
										}
									}
								}
							}

							// 檢查退款產品是否都在發貨單中
							canCreateReceivingNote := true
							for _, ir := range itemsRaw {
								im, ok := ir.(map[string]interface{})
								if !ok {
									continue
								}
								pid, _ := im["product_id"].(string)
								if pid == "" {
									continue
								}
								var refundQty float64
								switch v := im["quantity"].(type) {
								case float64:
									refundQty = v
								case float32:
									refundQty = float64(v)
								case int:
									refundQty = float64(v)
								case int64:
									refundQty = float64(v)
								case string:
									if parsed, err := strconv.ParseFloat(v, 64); err == nil {
										refundQty = parsed
									}
								}
								if refundQty <= 0 {
									continue
								}
								shippedQty := shippedQtyByProduct[pid]
								if shippedQty < refundQty {
									// 發貨數量不足，不生成入貨記錄
									canCreateReceivingNote = false
									log.Printf("退款產品 %s 發貨數量不足（發貨 %.0f < 退款 %.0f），不生成入貨記錄", pid, shippedQty, refundQty)
									break
								}
							}

							if !canCreateReceivingNote {
								// 跳過生成入貨記錄
								log.Printf("訂單 %s 退款單 %s：運送方式需要送貨但發貨單不包含退款產品，跳過生成入貨記錄", order.OrderNumber, refundNumber)
								continue
							}
						}

						// 使用退款單選擇的倉庫（退回倉庫）；若未選擇才 fallback 系統預設倉庫
						selectedWarehouseIDStr, _ := note["warehouse_id"].(string)
						receivingWarehouseID := selectedWarehouseIDStr
						if receivingWarehouseID == "" {
							receivingWarehouseID = defaultWarehouseID
						}

						receivingNoteIDStr, _ := note["receiving_note_id"].(string)
						// 檢查是否已經有有效的 receiving_note_id（冪等性）
						hasExistingReceivingNote := receivingNoteIDStr != ""
						// 如果沒有現有的入貨記錄，且有倉庫和產品明細，則創建新的
						if !hasExistingReceivingNote && receivingWarehouseID != "" && len(itemsRaw) > 0 {
							receivingNumber := fmt.Sprintf("RCV-%s", refundNumber)

							receivingNote := map[string]interface{}{
								"receiving_number": receivingNumber,
								"receiving_date":   refundDate.Format("2006-01-02"),
								"warehouse_id":     receivingWarehouseID,
								"notes":            fmt.Sprintf("訂單 %s 退款取回產品", order.OrderNumber),
								"items":            []interface{}{},
							}
							// 只取 product_id/quantity
							itemsForReceiving := make([]interface{}, 0)

							for _, ir := range itemsRaw {
								im, ok := ir.(map[string]interface{})
								if !ok {
									continue
								}
								pid, _ := im["product_id"].(string)
								if pid == "" {
									continue
								}
								// qty
								qty := 0.0
								switch v := im["quantity"].(type) {
								case float64:
									qty = v
								case float32:
									qty = float64(v)
								case int:
									qty = float64(v)
								case int64:
									qty = float64(v)
								case string:
									if parsed, err := strconv.ParseFloat(v, 64); err == nil {
										qty = parsed
									}
								default:
									if s := fmt.Sprintf("%v", v); s != "" {
										if parsed, err := strconv.ParseFloat(s, 64); err == nil {
											qty = parsed
										}
									}
								}
								if qty <= 0 {
									continue
								}
								qtyInt := int(math.Round(qty))
								if qtyInt <= 0 {
									continue
								}

								itemsForReceiving = append(itemsForReceiving, map[string]interface{}{
									"product_id": pid,
									"quantity":   float64(qtyInt),
								})
							}
							receivingNote["items"] = itemsForReceiving

							// 保存 receiving_note 到訂單的 extra_fields 中（用於查詢和顯示）
							if order.ExtraFields == nil {
								order.ExtraFields = models.JSONB(map[string]interface{}{})
							}
							orderFields := map[string]interface{}(order.ExtraFields)
							var receivingNotes []interface{}
							if existingNotes, exists := orderFields["receiving_notes"]; exists {
								if notesList, ok := existingNotes.([]interface{}); ok {
									receivingNotes = notesList
								}
							}
							if receivingNotes == nil {
								receivingNotes = make([]interface{}, 0)
							}
							receivingNotes = append(receivingNotes, receivingNote)
							orderFields["receiving_notes"] = receivingNotes
							order.ExtraFields = models.JSONB(orderFields)
							if err := database.DB.Model(&order).Update("extra_fields", order.ExtraFields).Error; err != nil {
								log.Printf("Failed to save receiving note to order extra_fields: %v", err)
							}

							// 保存 receiving_note_id 到退款單中（冪等性）
							note["receiving_note"] = receivingNote
							note["receiving_note_id"] = receivingNumber

							// 直接入庫
							whID, _ := uuid.Parse(receivingWarehouseID)
							for _, itAny := range itemsForReceiving {
								it, ok := itAny.(map[string]interface{})
								if !ok {
									continue
								}
								pidStr, _ := it["product_id"].(string)
								qtyF, _ := it["quantity"].(float64)
								if pidStr == "" || qtyF <= 0 {
									continue
								}
								pid, err := uuid.Parse(pidStr)
								if err != nil {
									continue
								}
								qtyInt := int(qtyF)

								// 入庫
								if whID != uuid.Nil && qtyInt > 0 {
									if err := utils.UpdateWarehouseStock(tenantID, pid, whID, qtyInt, "increase"); err != nil {
										log.Printf("Failed to update warehouse stock for refund return: %v", err)
									}
								}
							}
						}
					}

					// 3) Income（取回傭金 -> 收入記錄）
					if returnCommission {
						incomeIDStr, _ := note["income_id"].(string)
						if incomeIDStr == "" && commissionExpenseAmount > 0 {
							ratio := 0.0
							if order.TotalAmount > 0 {
								ratio = itemsTotal / order.TotalAmount
							}
							if ratio < 0 {
								ratio = 0
							}
							if ratio > 1 {
								ratio = 1
							}
							amt := commissionExpenseAmount * ratio
							if amt > 0 {
								desc := fmt.Sprintf("取回傭金（退款單 %s / 訂單 %s）", refundNumber, order.OrderNumber)
								relatedUserID := &userID
								if order.SalespersonID != nil {
									relatedUserID = order.SalespersonID
								}
								inc := models.Income{
									TenantID:      tenantID,
									RelatedUserID: relatedUserID,
									IncomeType:    "other",
									ReferenceID:   &order.ID,
									ReferenceType: "order_refund_commission",
									Category:      "refund_commission",
									Description:   desc,
									Amount:        amt,
									IncomeDate:    refundDate,
									PaymentMethod: "cash",
									Status:        "confirmed",
									Notes:         fmt.Sprintf("refund_note_index=%d", noteIndex),
									CreatedBy:     &userID,
									UpdatedBy:     &userID,
									CreatedAt:     time.Now(),
									UpdatedAt:     time.Now(),
									ExtraFields: models.JSONB(map[string]interface{}{
										"refund_number": refundNumber,
										"order_id":      order.ID.String(),
										"note_index":    noteIndex,
										"ratio":         ratio,
									}),
								}
								if err := database.DB.Create(&inc).Error; err == nil {
									note["income_id"] = inc.ID.String()
								}
							}
						}
					}
				}

				// 無論是否有 changed，都要保存 refund_notes 到 extra_fields
				// 因為即使沒有生成 expense/income/purchase_order，退款單數據本身也需要保存
				fields["refund_notes"] = notesList
				order.ExtraFields = models.JSONB(fields)
				database.DB.Model(&order).Update("extra_fields", order.ExtraFields)
			}
		}
	}

	// 處理介紹人積分獎勵（如果訂單狀態從非completed變為completed）
	if oldStatus != "completed" && order.Status == "completed" && order.CustomerID != nil {
		refCode := order.ReferralCode
		if refCode == "" {
			// 再嘗試從客戶資料取得
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *order.CustomerID).First(&customer).Error; err == nil {
				refCode = customer.ReferralCode
			}
		}
		if refCode != "" {
			processReferralBonus(tenantID, refCode, *order.CustomerID, "訂單", order.ID, order.OrderNumber, order.PointsEarned)
		}
	}

	// 處理會員等級自動升級（如果訂單狀態從非completed變為completed）
	if oldStatus != "completed" && order.Status == "completed" && order.CustomerID != nil {
		checkAndUpgradeMemberLevel(tenantID, *order.CustomerID, order.TotalAmount)
	}

	// 狀態轉 confirmed 時寄送訂單確認 email（有 email 即寄）
	if oldStatus != "confirmed" && order.Status == "confirmed" {
		emailTo := strings.TrimSpace(order.ContactEmail)
		customerName := strings.TrimSpace(order.ContactName)
		customerID := uuid.Nil
		if order.CustomerID != nil {
			customerID = *order.CustomerID
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *order.CustomerID).First(&customer).Error; err == nil {
				if emailTo == "" {
					emailTo = strings.TrimSpace(customer.Email)
				}
				if customerName == "" {
					customerName = strings.TrimSpace(customer.Name)
				}
			}
		}
		if emailTo != "" {
			if customerName == "" {
				customerName = "客戶"
			}
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				orderDate := order.OrderDate.Format("2006-01-02")
				if err := email.EnqueueOrderConfirmationEmail(
					tenantID,
					tenant.Subdomain,
					customerID,
					emailTo,
					customerName,
					order.OrderNumber,
					orderDate,
					order.TotalAmount,
					"order",
				); err != nil {
					log.Printf("Failed to enqueue order confirmation email: %v", err)
				}
			}
		}
	}

	database.DB.Where("id = ?", order.ID).Preload("Customer").Preload("OrderItems.Product").Preload("Labels").First(&order)

	// Auto-refresh business goals related to orders
	go RefreshActiveBusinessGoals(tenantID, []string{"order_count", "revenue", "product_sales_qty"})

	return c.JSON(order)
}

// DeleteOrder 刪除訂單（軟刪除：移到垃圾筒）
func DeleteOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order ID"})
	}

	// 檢查是否為系統預設資料
	if IsSystemDefault(database.DB, "orders", id, tenantID) {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot delete system default data"})
	}

	// 軟刪除：設置 trashed_at 時間
	result := database.DB.Model(&models.Order{}).Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).Update("trashed_at", time.Now())
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete order"})
	}
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}

	// Auto-refresh business goals related to orders
	go RefreshActiveBusinessGoals(tenantID, []string{"order_count", "revenue", "product_sales_qty"})

	return c.JSON(fiber.Map{
		"message": "Order moved to trash",
		"info":    "Data will be automatically deleted after 7 days",
	})
}

// ConvertQuotationToOrder 將報價單轉換為訂單（將狀態從 quotation 改為 confirmed）
func ConvertQuotationToOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	orderID := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	id, err := uuid.Parse(orderID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order ID"})
	}

	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}

	// 檢查是否為報價單
	if order.Status != "quotation" {
		return c.Status(400).JSON(fiber.Map{"error": "Only quotations can be converted to orders"})
	}

	// 更新狀態為已確認
	order.Status = "confirmed"
	order.UpdatedBy = &userID
	order.UpdatedAt = time.Now()

	if err := database.DB.Save(&order).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to convert quotation to order"})
	}

	return c.JSON(fiber.Map{"message": "Quotation converted to order successfully", "order": order})
}

// GetOrderReportData 獲取訂單報表數據
func GetOrderReportData(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	// 允許不帶日期：預設本月範圍（避免直接打 export/report API 400）
	if startDate == "" && endDate == "" {
		now := utils.NowInTenantTimezone(tenantID)
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		endDate = time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	} else if startDate == "" || endDate == "" {
		return c.Status(400).JSON(fiber.Map{"error": "start_date and end_date are required"})
	}

	// 解析日期（使用租戶時區）
	start, err := utils.ParseDateInTenantTimezone(tenantID, startDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid start_date format"})
	}
	end, err := utils.ParseDateInTenantTimezone(tenantID, endDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid end_date format"})
	}
	// 設置結束日期為當天結束時間
	end = end.Add(24*time.Hour - time.Second)

	// 構建查詢
	query := database.DB.Where("tenant_id = ? AND order_date >= ? AND order_date <= ?", tenantID, start, end)

	// 搜索過濾（訂單號或客戶名稱）
	if search := c.Query("search"); search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("order_number ILIKE ? OR EXISTS (SELECT 1 FROM customers WHERE customers.id = orders.customer_id AND (customers.name ILIKE ? OR customers.email ILIKE ? OR customers.phone ILIKE ?))",
			searchPattern, searchPattern, searchPattern, searchPattern)
	}

	var orders []models.Order
	if err := query.
		Preload("Customer").
		Preload("OrderItems").
		Order("order_date ASC").
		Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch orders"})
	}

	// 計算統計數據
	var totalOrders int = len(orders)
	var totalAmount float64 = 0
	var totalItems int = 0

	for _, order := range orders {
		totalAmount += order.TotalAmount
		totalItems += len(order.OrderItems)
	}

	avgAmount := 0.0
	if totalOrders > 0 {
		avgAmount = totalAmount / float64(totalOrders)
	}

	// 每日統計
	dailyStats := make(map[string]float64)
	statusStats := make(map[string]int)

	for _, order := range orders {
		dateKey := order.OrderDate.Format("2006-01-02")
		dailyStats[dateKey] += order.TotalAmount
		statusStats[order.Status]++
	}

	// 轉換為數組格式
	var dailyStatsArray []map[string]interface{}
	for date, amount := range dailyStats {
		dailyStatsArray = append(dailyStatsArray, map[string]interface{}{
			"date":   date,
			"amount": amount,
		})
	}

	var statusStatsArray []map[string]interface{}
	for status, count := range statusStats {
		statusStatsArray = append(statusStatsArray, map[string]interface{}{
			"status": status,
			"count":  count,
		})
	}

	return c.JSON(fiber.Map{
		"total_orders": totalOrders,
		"total_amount": totalAmount,
		"avg_amount":   avgAmount,
		"total_items":  totalItems,
		"daily_stats":  dailyStatsArray,
		"status_stats": statusStatsArray,
		"orders":       orders,
	})
}

// ExportOrdersToExcel 導出訂單到 Excel
func ExportOrdersToExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	// 允許不帶日期：預設本月範圍
	if startDate == "" && endDate == "" {
		now := utils.NowInTenantTimezone(tenantID)
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		endDate = time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	} else if startDate == "" || endDate == "" {
		return c.Status(400).JSON(fiber.Map{"error": "start_date and end_date are required"})
	}

	start, err := utils.ParseDateInTenantTimezone(tenantID, startDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid start_date format"})
	}
	end, err := utils.ParseDateInTenantTimezone(tenantID, endDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid end_date format"})
	}
	end = end.Add(24*time.Hour - time.Second)

	var orders []models.Order
	if err := database.DB.Where("tenant_id = ? AND order_date >= ? AND order_date <= ?", tenantID, start, end).
		Preload("Customer").
		Preload("OrderItems").
		Order("order_date ASC").
		Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch orders"})
	}

	// 創建 Excel 文件
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "訂單報表"
	f.NewSheet(sheetName)
	f.DeleteSheet("Sheet1")

	// 設置標題
	headers := []string{"訂單號", "客戶", "日期", "狀態", "總金額", "商品數", "備註"}
	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheetName, cell, header)
		style, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
		})
		f.SetCellStyle(sheetName, cell, cell, style)
	}

	// 填充數據
	for i, order := range orders {
		row := i + 2
		customerName := "無客戶"
		if order.Customer != nil {
			customerName = order.Customer.Name
		}

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), order.OrderNumber)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), customerName)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), order.OrderDate.Format("2006-01-02"))
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), order.Status)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), order.TotalAmount)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), len(order.OrderItems))
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), order.Notes)
	}

	// 設置列寬
	f.SetColWidth(sheetName, "A", "A", 20)
	f.SetColWidth(sheetName, "B", "B", 20)
	f.SetColWidth(sheetName, "C", "C", 15)
	f.SetColWidth(sheetName, "D", "D", 15)
	f.SetColWidth(sheetName, "E", "E", 15)
	f.SetColWidth(sheetName, "F", "F", 10)
	f.SetColWidth(sheetName, "G", "G", 30)

	// 設置響應頭
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=orders_%s_%s.xlsx", startDate, endDate))

	// 寫入響應
	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate Excel file"})
	}

	return nil
}

// ExportOrdersToPDF 導出訂單到 PDF
func ExportOrdersToPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	// 允許不帶日期：預設本月範圍
	if startDate == "" && endDate == "" {
		now := utils.NowInTenantTimezone(tenantID)
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		endDate = time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	} else if startDate == "" || endDate == "" {
		return c.Status(400).JSON(fiber.Map{"error": "start_date and end_date are required"})
	}

	start, err := utils.ParseDateInTenantTimezone(tenantID, startDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid start_date format"})
	}
	end, err := utils.ParseDateInTenantTimezone(tenantID, endDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid end_date format"})
	}
	end = end.Add(24*time.Hour - time.Second)

	var orders []models.Order
	if err := database.DB.Where("tenant_id = ? AND order_date >= ? AND order_date <= ?", tenantID, start, end).
		Preload("Customer").
		Preload("OrderItems").
		Order("order_date ASC").
		Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch orders"})
	}

	headers := []string{"訂單號", "客戶", "日期", "狀態", "總金額", "商品數"}
	rows := make([][]string, 0, len(orders))
	for _, order := range orders {
		customerName := "無客戶"
		if order.Customer != nil {
			customerName = order.Customer.Name
		}
		rows = append(rows, []string{
			order.OrderNumber,
			customerName,
			order.OrderDate.Format("2006-01-02"),
			order.Status,
			fmt.Sprintf("%.2f", order.TotalAmount),
			fmt.Sprintf("%d", len(order.OrderItems)),
		})
	}
	title := fmt.Sprintf("訂單報表（%s 至 %s）", startDate, endDate)
	pdfBytes, _ := utils.BuildTablePDFBytes(title, headers, rows)
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=orders_%s_%s.pdf", startDate, endDate))
	return c.Send(pdfBytes)
}
