package handlers

import (
	"fmt"
	"log"
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
)

// ============================================
// 採購單 (PurchaseOrder) CRUD
// ============================================

// parseAmount extracts amount from request payload (frontend may send number or string).
func parseAmount(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		if x == "" {
			return 0, true
		}
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// normalizeExpenseRecords filters out "empty" expense records from frontend
// (e.g. UI default row with amount=0). Only records with amount > 0 are valid.
func normalizeExpenseRecords(records []map[string]interface{}) []map[string]interface{} {
	if len(records) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(records))
	for _, r := range records {
		if r == nil {
			continue
		}
		amt, ok := parseAmount(r["amount"])
		if !ok {
			continue
		}
		if amt > 0 {
			out = append(out, r)
		}
	}
	return out
}

// GetLastSupplierQuotationPrice returns the latest quotation (orders.status=quotation)
// unit price for the same supplier (customer_id) and product.
// Query: supplier_id, product_id
func GetLastSupplierQuotationPrice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	supplierIDStr := strings.TrimSpace(c.Query("supplier_id"))
	productIDStr := strings.TrimSpace(c.Query("product_id"))
	if supplierIDStr == "" || productIDStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "supplier_id and product_id are required"})
	}

	supplierID, err := uuid.Parse(supplierIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid supplier_id"})
	}
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid product_id"})
	}

	// orders: quotations are stored as orders with status = 'quotation'
	// supplier is also a customer (customers.id)
	type row struct {
		UnitPrice   float64   `json:"unit_price"`
		OrderID     uuid.UUID `json:"order_id"`
		OrderNumber string    `json:"order_number"`
		OrderDate   time.Time `json:"order_date"`
	}
	var r row

	q := database.DB.Table("order_items").
		Select("order_items.unit_price, orders.id AS order_id, orders.order_number, orders.order_date").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.tenant_id = ? AND orders.status = ? AND orders.customer_id = ? AND orders.trashed_at IS NULL", tenantID, "quotation", supplierID).
		Where("order_items.product_id = ? AND order_items.trashed_at IS NULL", productID).
		Order("orders.order_date DESC, orders.created_at DESC, order_items.created_at DESC").
		Limit(1)

	if err := q.Scan(&r).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query last quotation price"})
	}
	if r.OrderID == uuid.Nil {
		return c.JSON(fiber.Map{"found": false})
	}

	return c.JSON(fiber.Map{
		"found":        true,
		"unit_price":   r.UnitPrice,
		"order_id":     r.OrderID,
		"order_number": r.OrderNumber,
		"order_date":   r.OrderDate,
	})
}

func GetPurchaseOrders(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	supplierID := c.Query("supplier_id")
	referenceType := c.Query("reference_type")
	referenceID := c.Query("reference_id")

	var purchaseOrders []models.PurchaseOrder
	query := database.DB.Where("tenant_id = ?", tenantID)

	if supplierID != "" {
		query = query.Where("supplier_id = ?", supplierID)
	}
	// 允許用 extra_fields 作為「關聯來源」過濾（例如：退款取回產品 -> 入貨記錄）
	// 這樣前端可以用 /purchase-orders?reference_type=...&reference_id=... 查到對應的入貨記錄
	if referenceType != "" && referenceID != "" {
		query = query.Where("extra_fields->>'reference_type' = ? AND extra_fields->>'reference_id' = ?", referenceType, referenceID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN purchase_order_label_relations ON purchase_orders.id = purchase_order_label_relations.purchase_order_id").
			Where("purchase_order_label_relations.label_id IN ?", labelIDs).
			Group("purchase_orders.id")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.PurchaseOrder{}).Count(&total)

	if err := query.Preload("Supplier").Preload("Labels").
		// 與 /orders 一致：預設按建立時間倒序（其次按單據日期倒序），分頁更穩定
		Offset(offset).Limit(limit).Order("created_at DESC, order_date DESC").
		Find(&purchaseOrders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  purchaseOrders,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetPurchaseOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var purchaseOrder models.PurchaseOrder

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("Supplier").Preload("PurchaseOrderItems").Preload("PurchaseOrderItems.Product").
		First(&purchaseOrder).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order not found"})
	}

	// 從 expenses 表加載支出記錄
	var expenses []models.Expense
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "purchase_order", purchaseOrder.ID).
		Order("expense_date DESC").
		Find(&expenses)

	// 將支出記錄轉換為前端格式
	expenseRecords := make([]map[string]interface{}, 0)
	for _, exp := range expenses {
		record := map[string]interface{}{
			"expense_id":       exp.ID.String(),
			"expense_date":     exp.ExpenseDate.Format("2006-01-02"),
			"amount":           exp.Amount,
			"payment_method":   exp.PaymentMethod,
			"bank_account_id":  "",
			"invoice_number":   "",
			"reference_number": "",
			"notes":            exp.Notes,
		}

		// 從 ExtraFields 中提取 payment_method_id, expense_number/invoice_number, reference_number
		if exp.ExtraFields != nil {
			if paymentMethodID, ok := exp.ExtraFields["payment_method_id"].(string); ok {
				record["payment_method_id"] = paymentMethodID
			}
			// 支出單號優先 expense_number，向後兼容 invoice_number
			if expenseNumber, ok := exp.ExtraFields["expense_number"].(string); ok && expenseNumber != "" {
				record["invoice_number"] = expenseNumber
			} else if invoiceNumber, ok := exp.ExtraFields["invoice_number"].(string); ok {
				record["invoice_number"] = invoiceNumber
			}
			if referenceNumber, ok := exp.ExtraFields["reference_number"].(string); ok {
				record["reference_number"] = referenceNumber
			}
		}

		if exp.BankAccountID != nil {
			record["bank_account_id"] = exp.BankAccountID.String()
		}

		expenseRecords = append(expenseRecords, record)
	}

	// 將支出記錄添加到響應中
	// 將 purchaseOrder 轉換為 map（簡化處理，直接構建響應）
	response := fiber.Map{
		"id":                     purchaseOrder.ID,
		"tenant_id":              purchaseOrder.TenantID,
		"supplier_id":            purchaseOrder.SupplierID,
		"order_number":           purchaseOrder.OrderNumber,
		"order_date":             purchaseOrder.OrderDate,
		"expected_delivery_date": purchaseOrder.ExpectedDeliveryDate,
		"total_amount":           purchaseOrder.TotalAmount,
		"discount_amount":        purchaseOrder.DiscountAmount,
		"tax_amount":             purchaseOrder.TaxAmount,
		"final_amount":           purchaseOrder.FinalAmount,
		"status":                 purchaseOrder.Status,
		"notes":                  purchaseOrder.Notes,
		"supplier":               purchaseOrder.Supplier,
		"purchase_items":         purchaseOrder.PurchaseOrderItems, // 使用 purchase_items 與前端一致
		"extra_fields":           purchaseOrder.ExtraFields,
		"created_at":             purchaseOrder.CreatedAt,
		"updated_at":             purchaseOrder.UpdatedAt,
		"expense_records":        expenseRecords, // 添加支出記錄
	}

	return c.JSON(response)
}

type CreatePurchaseOrderRequest struct {
	SupplierID           *string                  `json:"supplier_id"`
	OrderNumber          string                   `json:"order_number"`
	OrderDate            string                   `json:"order_date"`
	ExpectedDeliveryDate *string                  `json:"expected_delivery_date,omitempty"`
	TotalAmount          float64                  `json:"total_amount"`
	DiscountAmount       float64                  `json:"discount_amount"`
	TaxAmount            float64                  `json:"tax_amount"`
	FinalAmount          float64                  `json:"final_amount"`
	Status               string                   `json:"status"`
	Notes                string                   `json:"notes"`
	ExpenseRecords       []map[string]interface{} `json:"expense_records"`
	ExtraFields          map[string]interface{}   `json:"extra_fields,omitempty"`
	PurchaseItems        []struct {
		ProductID         *string                  `json:"product_id"`
		Quantity          int                      `json:"quantity"`
		UnitPrice         float64                  `json:"unit_price"`
		DiscountAmount    float64                  `json:"discount_amount"`
		TaxRate           float64                  `json:"tax_rate"`
		Notes             string                   `json:"notes"`
		ProductAttributes []map[string]interface{} `json:"product_attributes,omitempty"`
	} `json:"purchase_items"`
}

func CreatePurchaseOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var req CreatePurchaseOrderRequest
	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse purchase order request: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	// 解析日期（使用租戶時區）
	orderDate, err := utils.ParseDateInTenantTimezone(tenantID, req.OrderDate)
	if err != nil {
		orderDate = utils.NowInTenantTimezone(tenantID)
	}

	var expectedDeliveryDate *time.Time
	if req.ExpectedDeliveryDate != nil && *req.ExpectedDeliveryDate != "" {
		if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, *req.ExpectedDeliveryDate); err == nil {
			expectedDeliveryDate = &parsedDate
		}
	}

	// 解析供應商ID
	var supplierID *uuid.UUID
	if req.SupplierID != nil && *req.SupplierID != "" {
		if parsedID, err := uuid.Parse(*req.SupplierID); err == nil {
			// 驗證供應商是否存在且屬於當前租戶
			var supplier models.Supplier
			if err := database.DB.Where("id = ? AND tenant_id = ?", parsedID, tenantID).First(&supplier).Error; err != nil {
				log.Printf("[CreatePurchaseOrder] Invalid supplier ID: %v, error: %v", parsedID, err)
				return c.Status(400).JSON(fiber.Map{"error": "Invalid supplier ID: supplier not found or does not belong to this tenant"})
			}
			supplierID = &parsedID
			log.Printf("[CreatePurchaseOrder] Validated supplier ID: %v", *supplierID)
		} else {
			log.Printf("[CreatePurchaseOrder] Invalid supplier ID format: %v, error: %v", *req.SupplierID, err)
			return c.Status(400).JSON(fiber.Map{"error": "Invalid supplier ID format"})
		}
	}

	// 處理 ExtraFields（不再包含 expense_records）
	extraFields := make(map[string]interface{})
	if req.ExtraFields != nil {
		for k, v := range req.ExtraFields {
			// 跳過 expense_records，不再保存到 ExtraFields
			if k != "expense_records" {
				extraFields[k] = v
			}
		}
	}
	settings := getInventorySettingsForTenant(tenantID)
	applyAutoCompleteInventoryNotes(settings, extraFields, "receiving_notes")

	purchaseOrder := models.PurchaseOrder{
		TenantID:             tenantID,
		SupplierID:           supplierID,
		OrderNumber:          req.OrderNumber,
		OrderDate:            orderDate,
		ExpectedDeliveryDate: expectedDeliveryDate,
		TotalAmount:          req.TotalAmount,
		DiscountAmount:       req.DiscountAmount,
		TaxAmount:            req.TaxAmount,
		FinalAmount:          req.FinalAmount,
		Status:               req.Status,
		Notes:                req.Notes,
		ExtraFields:          models.JSONB(extraFields),
	}

	// 生成訂單號
	if purchaseOrder.OrderNumber == "" {
		purchaseOrder.OrderNumber = "PO-" + time.Now().Format("20060102150405")
		// 檢查是否已存在，如果存在則重新生成
		for {
			var existing models.PurchaseOrder
			if err := database.DB.Where("order_number = ?", purchaseOrder.OrderNumber).First(&existing).Error; err != nil {
				break
			}
			purchaseOrder.OrderNumber = "PO-" + time.Now().Format("20060102150405")
		}
	}

	// 計算總金額
	var subtotal float64
	for _, item := range req.PurchaseItems {
		itemSubtotal := float64(item.Quantity) * item.UnitPrice
		itemDiscount := item.DiscountAmount
		itemTax := (itemSubtotal - itemDiscount) * item.TaxRate / 100
		subtotal += itemSubtotal - itemDiscount + itemTax
	}
	purchaseOrder.TotalAmount = subtotal
	purchaseOrder.FinalAmount = subtotal - purchaseOrder.DiscountAmount + purchaseOrder.TaxAmount

	// 在插入前再次驗證 supplier_id（如果提供）
	if purchaseOrder.SupplierID != nil {
		var supplierCheck models.Supplier
		if err := database.DB.Where("id = ? AND tenant_id = ?", *purchaseOrder.SupplierID, tenantID).First(&supplierCheck).Error; err != nil {
			log.Printf("[CreatePurchaseOrder] ❌ Final validation failed: supplier_id %v does NOT exist in suppliers table, error: %v - REJECTING INSERT", *purchaseOrder.SupplierID, err)
			return c.Status(400).JSON(fiber.Map{"error": "Invalid supplier ID: supplier not found or does not belong to this tenant"})
		}
		log.Printf("[CreatePurchaseOrder] ✅ Final validation passed: supplier_id %v exists in suppliers table", *purchaseOrder.SupplierID)
	}

	if err := database.DB.Create(&purchaseOrder).Error; err != nil {
		log.Printf("[CreatePurchaseOrder] Failed to create purchase order: %v, purchaseOrder data: %+v", err, purchaseOrder)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create purchase order: " + err.Error()})
	}

	// 創建採購單明細
	for _, itemReq := range req.PurchaseItems {
		if itemReq.ProductID == nil {
			continue
		}
		productID, err := uuid.Parse(*itemReq.ProductID)
		if err != nil {
			continue
		}

		itemSubtotal := float64(itemReq.Quantity) * itemReq.UnitPrice
		itemDiscount := itemReq.DiscountAmount
		itemTax := (itemSubtotal - itemDiscount) * itemReq.TaxRate / 100
		totalAmount := itemSubtotal - itemDiscount + itemTax

		item := models.PurchaseOrderItem{
			PurchaseOrderID: purchaseOrder.ID,
			ProductID:       &productID,
			Quantity:        itemReq.Quantity,
			UnitPrice:       itemReq.UnitPrice,
			DiscountAmount:  itemDiscount,
			TaxRate:         itemReq.TaxRate,
			TotalAmount:     totalAmount,
			Notes:           itemReq.Notes,
		}

		if err := database.DB.Create(&item).Error; err != nil {
			// 記錄錯誤但不中斷流程
			log.Printf("Failed to create purchase order item: %v", err)
		}
	}

	// 處理支出記錄（從頂級字段 expense_records）
	userID := middleware.GetUserID(c)
	validExpenseRecords := normalizeExpenseRecords(req.ExpenseRecords)
	if len(validExpenseRecords) > 0 {
		// 獲取供應商名稱
		var vendor string
		if supplierID != nil {
			var supplier models.Supplier
			if err := database.DB.Where("id = ? AND tenant_id = ?", supplierID, tenantID).First(&supplier).Error; err == nil {
				vendor = supplier.Name
			}
		}

		for _, record := range validExpenseRecords {
			// 解析支出記錄（使用租戶時區）
			expenseDateStr, _ := record["expense_date"].(string)
			if expenseDateStr == "" {
				expenseDateStr, _ = record["payment_date"].(string) // 向後兼容
			}
			expenseDate, err := utils.ParseDateInTenantTimezone(tenantID, expenseDateStr)
			if err != nil {
				expenseDate = utils.NowInTenantTimezone(tenantID)
			}

			amount, _ := parseAmount(record["amount"])
			if amount > 0 {
				paymentMethod, _ := record["payment_method"].(string)
				paymentMethodIDStr, _ := record["payment_method_id"].(string)
				var bankAccountID *uuid.UUID
				if bankAccountIDStr, ok := record["bank_account_id"].(string); ok && bankAccountIDStr != "" {
					if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
						bankAccountID = &parsedID
					}
				}

				// 支出單號：優先用 expense_number，其次使用前端沿用欄位 invoice_number
				expenseNumber, _ := record["expense_number"].(string)
				if expenseNumber == "" {
					expenseNumber, _ = record["invoice_number"].(string)
				}
				referenceNumber, _ := record["reference_number"].(string)
				notes, _ := record["notes"].(string)

				// 若沒有支出單號或是誤用 INV-*，由後端生成並預留 EXP-*
				if expenseNumber == "" || strings.HasPrefix(expenseNumber, "INV-") || !strings.HasPrefix(expenseNumber, "EXP-") {
					if n, err := reserveNextNumber(tenantID, "expense_number", "expenses"); err == nil {
						expenseNumber = n
					}
				}

				// 描述只用支出單號碼（不是發票）
				description := expenseNumber
				if description == "" {
					description = fmt.Sprintf("採購單 %s 的支出", purchaseOrder.OrderNumber)
				}

				// 創建支出記錄
				expense := models.Expense{
					TenantID:      tenantID,
					RelatedUserID: &userID,
					ExpenseType:   "purchase",
					ReferenceID:   &purchaseOrder.ID,
					ReferenceType: "purchase_order",
					Category:      "purchase",
					Description:   description,
					Amount:        amount,
					ExpenseDate:   expenseDate,
					PaymentMethod: paymentMethod,
					BankAccountID: bankAccountID,
					Vendor:        vendor,
					Status:        "confirmed",
					Notes:         notes,
					CreatedBy:     &userID,
					UpdatedBy:     &userID,
					CreatedAt:     time.Now(),
					UpdatedAt:     time.Now(),
					ExtraFields: models.JSONB(map[string]interface{}{
						"payment_method_id": paymentMethodIDStr,
						"expense_number":    expenseNumber,
						"invoice_number":    expenseNumber, // 向後兼容
						"reference_number":  referenceNumber,
					}),
				}

				if err := database.DB.Create(&expense).Error; err != nil {
					log.Printf("Failed to create expense record: %v", err)
					// 不中斷流程，只記錄錯誤
				}
			}
		}
	} else {
		// 自動生成採購單支出記錄（預設開啟）
		auto := getDocumentAutoSettingsForTenant(tenantID)
		shouldAuto := auto.AutoGeneratePurchaseOrderExpense && purchaseOrder.FinalAmount > 0
		if shouldAuto {
			// 預設支出付款方式/付款賬戶
			defPayMethod := defaultPaymentMethodCode(tenantID)
			if defPayMethod == "" {
				defPayMethod = "cash"
			}
			defBankAccID := defaultPaymentBankAccountID(tenantID)

			// 嘗試取得 payment_method_id（僅供前端顯示）
			paymentMethodID := ""
			{
				var pm models.PaymentMethod
				if err := database.DB.Where("tenant_id = ? AND status = ? AND code = ?", tenantID, "active", defPayMethod).First(&pm).Error; err == nil && pm.ID != uuid.Nil {
					paymentMethodID = pm.ID.String()
				}
			}

			// 支出單號（EXP-YYYYMMDD-###，會預留避免重複）
			expenseNumber := ""
			if n, err := reserveNextNumber(tenantID, "expense_number", "expenses"); err == nil {
				expenseNumber = n
			}

			// Vendor
			vendor := ""
			if supplierID != nil {
				var supplier models.Supplier
				if err := database.DB.Where("id = ? AND tenant_id = ?", supplierID, tenantID).First(&supplier).Error; err == nil {
					vendor = supplier.Name
				}
			}

			// 描述只用支出單號碼（不是發票）
			description := expenseNumber
			if description == "" {
				description = fmt.Sprintf("採購單 %s 的支出", purchaseOrder.OrderNumber)
			}

			expense := models.Expense{
				TenantID:      tenantID,
				RelatedUserID: &userID,
				ExpenseType:   "purchase",
				ReferenceID:   &purchaseOrder.ID,
				ReferenceType: "purchase_order",
				Category:      "purchase",
				Description:   description,
				Amount:        purchaseOrder.FinalAmount,
				ExpenseDate:   purchaseOrder.OrderDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defBankAccID,
				Vendor:        vendor,
				Status:        "confirmed",
				Notes:         "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				ExtraFields: models.JSONB(map[string]interface{}{
					"payment_method_id": paymentMethodID,
					"expense_number":    expenseNumber,
					"invoice_number":    expenseNumber, // 向後兼容（前端欄位仍沿用 invoice_number 顯示支出單號）
					"purchase_order_id": purchaseOrder.ID.String(),
				}),
			}
			if err := database.DB.Create(&expense).Error; err != nil {
				log.Printf("Failed to auto-generate purchase expense record: %v", err)
			}
		}
	}

	// 處理收貨單並更新庫存（從 receiving_notes）
	if purchaseOrder.ExtraFields != nil {
		fields := map[string]interface{}(purchaseOrder.ExtraFields)
		if receivingNotes, exists := fields["receiving_notes"]; exists {
			if notesList, ok := receivingNotes.([]interface{}); ok {
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
													// 收貨：增加庫存
													if err := utils.UpdateWarehouseStock(tenantID, productID, warehouseID, int(quantity), "increase"); err != nil {
														log.Printf("Failed to update warehouse stock for receiving: %v", err)
														// 不中斷流程，只記錄錯誤
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

	// 發送採購單確認 email（狀態已確認，寄租戶 email）
	if purchaseOrder.Status == "confirmed" {
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			emailTo := ""
			if tenant.ExtraFields != nil {
				if v, ok := tenant.ExtraFields["email"].(string); ok {
					emailTo = strings.TrimSpace(v)
				} else if v, ok := tenant.ExtraFields["contact_email"].(string); ok {
					emailTo = strings.TrimSpace(v)
				}
			}
			if emailTo != "" {
				orderDate := purchaseOrder.OrderDate.Format("2006-01-02")
				if err := email.EnqueueOrderConfirmationEmail(
					tenantID,
					tenant.Subdomain,
					uuid.Nil,
					emailTo,
					tenant.Name,
					purchaseOrder.OrderNumber,
					orderDate,
					purchaseOrder.FinalAmount,
					"purchase_order",
				); err != nil {
					log.Printf("Failed to enqueue purchase order confirmation email: %v", err)
				}
			}
		}
	}

	return c.Status(201).JSON(purchaseOrder)
}

func UpdatePurchaseOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var req CreatePurchaseOrderRequest

	var purchaseOrder models.PurchaseOrder
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&purchaseOrder).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order not found"})
	}
	oldStatus := purchaseOrder.Status

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 解析日期（使用租戶時區）
	orderDate, err := utils.ParseDateInTenantTimezone(tenantID, req.OrderDate)
	if err != nil {
		orderDate = purchaseOrder.OrderDate // 如果解析失敗，保持原值
	}

	var expectedDeliveryDate *time.Time
	if req.ExpectedDeliveryDate != nil && *req.ExpectedDeliveryDate != "" {
		if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, *req.ExpectedDeliveryDate); err == nil {
			expectedDeliveryDate = &parsedDate
		}
	}

	// 解析供應商ID
	var supplierID *uuid.UUID
	if req.SupplierID != nil && *req.SupplierID != "" {
		if parsedID, err := uuid.Parse(*req.SupplierID); err == nil {
			// 驗證供應商是否存在且屬於當前租戶
			var supplier models.Supplier
			if err := database.DB.Where("id = ? AND tenant_id = ?", parsedID, tenantID).First(&supplier).Error; err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid supplier ID: supplier not found or does not belong to this tenant"})
			}
			supplierID = &parsedID
		} else {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid supplier ID format"})
		}
	} else {
		// 如果前端傳送空字符串或 null，保持原來的 supplier_id（不允許清除，避免外鍵約束錯誤）
		// 這確保了如果採購單已經有供應商，不會因為前端傳送空值而導致外鍵約束錯誤
		supplierID = purchaseOrder.SupplierID
	}

	// 計算總金額
	var subtotal float64
	for _, item := range req.PurchaseItems {
		itemSubtotal := float64(item.Quantity) * item.UnitPrice
		itemDiscount := item.DiscountAmount
		itemTax := (itemSubtotal - itemDiscount) * item.TaxRate / 100
		subtotal += itemSubtotal - itemDiscount + itemTax
	}

	// 更新字段（使用 Updates 方法，只更新非零值字段，避免外鍵約束錯誤）
	updates := map[string]interface{}{
		"order_number":    req.OrderNumber,
		"order_date":      orderDate,
		"total_amount":    subtotal,
		"discount_amount": req.DiscountAmount,
		"tax_amount":      req.TaxAmount,
		"final_amount":    subtotal - req.DiscountAmount + req.TaxAmount,
		"status":          req.Status,
		"notes":           req.Notes,
	}
	if expectedDeliveryDate != nil {
		updates["expected_delivery_date"] = expectedDeliveryDate
	}
	// 只有在 supplierID 不為 nil 時才更新（如果為 nil，保持原值）
	if supplierID != nil {
		updates["supplier_id"] = supplierID
	}

	// 處理 ExtraFields（不再包含 expense_records）
	extraFields := make(map[string]interface{})
	if req.ExtraFields != nil {
		for k, v := range req.ExtraFields {
			// 跳過 expense_records，不再保存到 ExtraFields
			if k != "expense_records" {
				extraFields[k] = v
			}
		}
	}
	settings := getInventorySettingsForTenant(tenantID)
	applyAutoCompleteInventoryNotes(settings, extraFields, "receiving_notes")
	// 更新 ExtraFields（合併到 updates 中）
	if len(extraFields) > 0 {
		// 重要：採購單 extra_fields 內還會保存 cancellation_shipping_notes 等衍生資料
		// 若直接用 req.ExtraFields 覆蓋，會把這些欄位清掉，導致「取消採購單出貨記錄」消失
		merged := make(map[string]interface{})
		if purchaseOrder.ExtraFields != nil {
			for k, v := range map[string]interface{}(purchaseOrder.ExtraFields) {
				merged[k] = v
			}
		}
		for k, v := range extraFields {
			merged[k] = v
		}
		updates["extra_fields"] = models.JSONB(merged)
	}

	// 執行更新（一次性更新所有字段，包括 extra_fields）
	if err := database.DB.Model(&purchaseOrder).Updates(updates).Error; err != nil {
		log.Printf("Failed to update purchase order: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update purchase order: " + err.Error()})
	}

	// 重新載入採購單以獲取最新數據
	if err := database.DB.Where("id = ?", purchaseOrder.ID).Preload("Supplier").Preload("PurchaseOrderItems").Preload("PurchaseOrderItems.Product").First(&purchaseOrder).Error; err != nil {
		log.Printf("Failed to reload purchase order: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload purchase order"})
	}

	// 狀態轉 confirmed 時寄送採購單確認 email（寄租戶 email）
	if oldStatus != "confirmed" && purchaseOrder.Status == "confirmed" {
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			emailTo := ""
			if tenant.ExtraFields != nil {
				if v, ok := tenant.ExtraFields["email"].(string); ok {
					emailTo = strings.TrimSpace(v)
				} else if v, ok := tenant.ExtraFields["contact_email"].(string); ok {
					emailTo = strings.TrimSpace(v)
				}
			}
			if emailTo != "" {
				orderDate := purchaseOrder.OrderDate.Format("2006-01-02")
				if err := email.EnqueueOrderConfirmationEmail(
					tenantID,
					tenant.Subdomain,
					uuid.Nil,
					emailTo,
					tenant.Name,
					purchaseOrder.OrderNumber,
					orderDate,
					purchaseOrder.FinalAmount,
					"purchase_order",
				); err != nil {
					log.Printf("Failed to enqueue purchase order confirmation email: %v", err)
				}
			}
		}
	}

	// 處理收貨單並更新庫存（從 receiving_notes）
	if purchaseOrder.ExtraFields != nil {
		fields := map[string]interface{}(purchaseOrder.ExtraFields)
		if receivingNotes, exists := fields["receiving_notes"]; exists {
			if notesList, ok := receivingNotes.([]interface{}); ok {
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
													// 收貨：增加庫存
													if err := utils.UpdateWarehouseStock(tenantID, productID, warehouseID, int(quantity), "increase"); err != nil {
														log.Printf("Failed to update warehouse stock for receiving: %v", err)
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

	// 刪除舊的採購單明細
	database.DB.Where("purchase_order_id = ?", purchaseOrder.ID).Delete(&models.PurchaseOrderItem{})

	// 創建新的採購單明細
	for _, itemReq := range req.PurchaseItems {
		if itemReq.ProductID == nil {
			continue
		}
		productID, err := uuid.Parse(*itemReq.ProductID)
		if err != nil {
			continue
		}

		itemSubtotal := float64(itemReq.Quantity) * itemReq.UnitPrice
		itemDiscount := itemReq.DiscountAmount
		itemTax := (itemSubtotal - itemDiscount) * itemReq.TaxRate / 100
		totalAmount := itemSubtotal - itemDiscount + itemTax

		item := models.PurchaseOrderItem{
			PurchaseOrderID: purchaseOrder.ID,
			ProductID:       &productID,
			Quantity:        itemReq.Quantity,
			UnitPrice:       itemReq.UnitPrice,
			DiscountAmount:  itemDiscount,
			TaxRate:         itemReq.TaxRate,
			TotalAmount:     totalAmount,
			Notes:           itemReq.Notes,
		}

		if err := database.DB.Create(&item).Error; err != nil {
			log.Printf("Failed to create purchase order item: %v", err)
		}
	}

	// 處理支出記錄（從頂級字段 expense_records）
	userID := middleware.GetUserID(c)
	validExpenseRecords := normalizeExpenseRecords(req.ExpenseRecords)

	// 獲取現有的支出記錄
	var existingExpenses []models.Expense
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "purchase_order", purchaseOrder.ID).Find(&existingExpenses)
	existingExpenseIDs := make(map[uuid.UUID]bool)
	for _, exp := range existingExpenses {
		existingExpenseIDs[exp.ID] = true
	}

	// 獲取供應商名稱
	var vendor string
	if supplierID != nil {
		var supplier models.Supplier
		if err := database.DB.Where("id = ? AND tenant_id = ?", supplierID, tenantID).First(&supplier).Error; err == nil {
			vendor = supplier.Name
		}
	}

	// 記錄哪些 expense_id 在新的支出記錄中
	keptExpenseIDs := make(map[uuid.UUID]bool)

	// 若沒有支出記錄：依 document-auto-settings 決定是否自動生成
	if len(validExpenseRecords) == 0 {
		auto := getDocumentAutoSettingsForTenant(tenantID)
		shouldAuto := auto.AutoGeneratePurchaseOrderExpense && purchaseOrder.FinalAmount > 0
		if shouldAuto {
			// 若已存在支出，保留（不刪除），避免因前端沒帶 expense_records 而被清掉
			if len(existingExpenses) > 0 {
				for _, exp := range existingExpenses {
					keptExpenseIDs[exp.ID] = true
				}
			} else {
				defPayMethod := defaultPaymentMethodCode(tenantID)
				if defPayMethod == "" {
					defPayMethod = "cash"
				}
				defBankAccID := defaultPaymentBankAccountID(tenantID)
				paymentMethodID := ""
				{
					var pm models.PaymentMethod
					if err := database.DB.Where("tenant_id = ? AND status = ? AND code = ?", tenantID, "active", defPayMethod).First(&pm).Error; err == nil && pm.ID != uuid.Nil {
						paymentMethodID = pm.ID.String()
					}
				}
				expenseNumber := ""
				if n, err := reserveNextNumber(tenantID, "expense_number", "expenses"); err == nil {
					expenseNumber = n
				}
				description := expenseNumber
				if description == "" {
					description = fmt.Sprintf("採購單 %s 的支出", purchaseOrder.OrderNumber)
				}
				expense := models.Expense{
					TenantID:      tenantID,
					RelatedUserID: &userID,
					ExpenseType:   "purchase",
					ReferenceID:   &purchaseOrder.ID,
					ReferenceType: "purchase_order",
					Category:      "purchase",
					Description:   description,
					Amount:        purchaseOrder.FinalAmount,
					ExpenseDate:   purchaseOrder.OrderDate,
					PaymentMethod: defPayMethod,
					BankAccountID: defBankAccID,
					Vendor:        vendor,
					Status:        "confirmed",
					Notes:         "",
					CreatedBy:     &userID,
					UpdatedBy:     &userID,
					CreatedAt:     time.Now(),
					UpdatedAt:     time.Now(),
					ExtraFields: models.JSONB(map[string]interface{}{
						"payment_method_id": paymentMethodID,
						"expense_number":    expenseNumber,
						"invoice_number":    expenseNumber, // 向後兼容
						"purchase_order_id": purchaseOrder.ID.String(),
					}),
				}
				if err := database.DB.Create(&expense).Error; err == nil {
					keptExpenseIDs[expense.ID] = true
				}
			}
		}
	}

	// 處理新的支出記錄
	if len(validExpenseRecords) > 0 {
		for _, record := range validExpenseRecords {
			// 處理金額（可能是 float64 或字符串）
			var amount float64
			amountRaw, exists := record["amount"]
			if !exists {
				continue
			}

			switch v := amountRaw.(type) {
			case float64:
				amount = v
			case string:
				if parsed, err := strconv.ParseFloat(v, 64); err == nil {
					amount = parsed
				} else {
					continue
				}
			default:
				continue
			}

			if amount <= 0 {
				continue
			}

			// 檢查是否有 expense_id（更新現有記錄）
			expenseIDStr, hasExpenseID := record["expense_id"].(string)
			if hasExpenseID && expenseIDStr != "" {
				expenseID, err := uuid.Parse(expenseIDStr)
				if err == nil {
					// 更新現有支出記錄
					var existingExpense models.Expense
					if err := database.DB.Where("id = ? AND tenant_id = ?", expenseID, tenantID).First(&existingExpense).Error; err == nil {
						expenseDateStr, _ := record["expense_date"].(string)
						if expenseDateStr == "" {
							expenseDateStr, _ = record["payment_date"].(string) // 向後兼容
						}
						expenseDate, err := utils.ParseDateInTenantTimezone(tenantID, expenseDateStr)
						if err != nil {
							expenseDate = utils.NowInTenantTimezone(tenantID)
						}

						paymentMethod, _ := record["payment_method"].(string)
						paymentMethodIDStr, _ := record["payment_method_id"].(string)
						var bankAccountID *uuid.UUID
						if bankAccountIDStr, ok := record["bank_account_id"].(string); ok && bankAccountIDStr != "" {
							if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
								bankAccountID = &parsedID
							}
						}

						invoiceNumber, _ := record["invoice_number"].(string)
						referenceNumber, _ := record["reference_number"].(string)
						notes, _ := record["notes"].(string)

						description := fmt.Sprintf("採購單 %s 的支出", purchaseOrder.OrderNumber)
						if invoiceNumber != "" {
							description = invoiceNumber
						}

						existingExpense.Amount = amount
						existingExpense.ExpenseDate = expenseDate
						existingExpense.PaymentMethod = paymentMethod
						existingExpense.BankAccountID = bankAccountID
						existingExpense.Vendor = vendor
						existingExpense.Notes = notes
						existingExpense.Description = description
						existingExpense.UpdatedBy = &userID
						existingExpense.UpdatedAt = time.Now()
						existingExpense.ExtraFields = models.JSONB(map[string]interface{}{
							"payment_method_id": paymentMethodIDStr,
							"invoice_number":    invoiceNumber,
							"reference_number":  referenceNumber,
						})

						if err := database.DB.Save(&existingExpense).Error; err == nil {
							keptExpenseIDs[existingExpense.ID] = true
						}
					}
				}
			} else {
				// 創建新的支出記錄
				expenseDateStr, _ := record["expense_date"].(string)
				if expenseDateStr == "" {
					expenseDateStr, _ = record["payment_date"].(string) // 向後兼容
				}
				expenseDate, err := utils.ParseDateInTenantTimezone(tenantID, expenseDateStr)
				if err != nil {
					expenseDate = utils.NowInTenantTimezone(tenantID)
				}

				paymentMethod, _ := record["payment_method"].(string)
				paymentMethodIDStr, _ := record["payment_method_id"].(string)
				var bankAccountID *uuid.UUID
				if bankAccountIDStr, ok := record["bank_account_id"].(string); ok && bankAccountIDStr != "" {
					if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
						bankAccountID = &parsedID
					}
				}

				invoiceNumber, _ := record["invoice_number"].(string)
				referenceNumber, _ := record["reference_number"].(string)
				notes, _ := record["notes"].(string)

				description := fmt.Sprintf("採購單 %s 的支出", purchaseOrder.OrderNumber)
				if invoiceNumber != "" {
					description = invoiceNumber
				}

				expense := models.Expense{
					TenantID:      tenantID,
					RelatedUserID: &userID,
					ExpenseType:   "purchase",
					ReferenceID:   &purchaseOrder.ID,
					ReferenceType: "purchase_order",
					Category:      "purchase",
					Description:   description,
					Amount:        amount,
					ExpenseDate:   expenseDate,
					PaymentMethod: paymentMethod,
					BankAccountID: bankAccountID,
					Vendor:        vendor,
					Status:        "confirmed",
					Notes:         notes,
					CreatedBy:     &userID,
					UpdatedBy:     &userID,
					CreatedAt:     time.Now(),
					UpdatedAt:     time.Now(),
					ExtraFields: models.JSONB(map[string]interface{}{
						"payment_method_id": paymentMethodIDStr,
						"invoice_number":    invoiceNumber,
						"reference_number":  referenceNumber,
					}),
				}

				if err := database.DB.Create(&expense).Error; err == nil {
					keptExpenseIDs[expense.ID] = true
				}
			}
		}
	}

	// 刪除不在新支出記錄中的 expense 記錄
	for expenseID := range existingExpenseIDs {
		if !keptExpenseIDs[expenseID] {
			database.DB.Where("id = ? AND tenant_id = ?", expenseID, tenantID).Delete(&models.Expense{})
		}
	}

	// ============================================
	// 取消採購單（cancellation_notes）
	// - 拿回採購錢 -> incomes（reference_type=purchase_order_cancellation）
	// - 退回貨物 -> shipping_notes（出庫，減少庫存）
	// 以 cancellation_notes 內的 *_id 做冪等，避免重複生成
	// ============================================
	if req.ExtraFields != nil {
		fields := req.ExtraFields
		if cancellationNotes, exists := fields["cancellation_notes"]; exists {
			if notesList, ok := cancellationNotes.([]interface{}); ok {
				for noteIndex, noteInterface := range notesList {
					note, ok := noteInterface.(map[string]interface{})
					if !ok {
						continue
					}

					cancellationNumber, _ := note["cancellation_number"].(string)
					cancellationDateStr, _ := note["cancellation_date"].(string)
					returnMoney, _ := note["return_money"].(bool)
					returnProducts, _ := note["return_products"].(bool)
					cancellationTotalAmount, _ := note["cancellation_total_amount"].(float64)
					notes, _ := note["notes"].(string)

					if cancellationNumber == "" {
						continue
					}

					// 解析取消日期
					var cancellationDate time.Time
					if cancellationDateStr != "" {
						if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, cancellationDateStr); err == nil {
							cancellationDate = parsedDate
						} else {
							cancellationDate = purchaseOrder.OrderDate
						}
					} else {
						cancellationDate = purchaseOrder.OrderDate
					}

					// 1) Income（拿回採購錢 -> 收入記錄）
					if returnMoney && cancellationTotalAmount > 0 {
						incomeIDStr, _ := note["income_id"].(string)
						if incomeIDStr == "" {
							// 創建新的 income 記錄
							description := fmt.Sprintf("取消採購單 %s（採購單 %s）", cancellationNumber, purchaseOrder.OrderNumber)
							relatedUserID := &userID
							// 可以從 extra_fields 中獲取 purchaser_id
							if purchaseOrder.ExtraFields != nil {
								if purchaserIDStr, ok := purchaseOrder.ExtraFields["purchaser_id"].(string); ok && purchaserIDStr != "" {
									if parsedID, err := uuid.Parse(purchaserIDStr); err == nil {
										relatedUserID = &parsedID
									}
								}
							}

							inc := models.Income{
								TenantID:      tenantID,
								RelatedUserID: relatedUserID,
								IncomeType:    "other",
								ReferenceID:   &purchaseOrder.ID,
								ReferenceType: "purchase_order_cancellation",
								Category:      "purchase_cancellation",
								Description:   description,
								Amount:        cancellationTotalAmount,
								IncomeDate:    cancellationDate,
								PaymentMethod: "cash", // 默認現金，可以從前端傳入
								Status:        "confirmed",
								Notes:         notes,
								CreatedBy:     &userID,
								UpdatedBy:     &userID,
								CreatedAt:     time.Now(),
								UpdatedAt:     time.Now(),
								ExtraFields: models.JSONB(map[string]interface{}{
									"cancellation_number": cancellationNumber,
									"purchase_order_id":   purchaseOrder.ID.String(),
									"note_index":          noteIndex,
								}),
							}
							if err := database.DB.Create(&inc).Error; err == nil {
								note["income_id"] = inc.ID.String()
							}
						}
					}

					// 2) Shipping Note（退回貨物 -> 出庫，減少庫存）
					if returnProducts {
						// 驗證：如果選擇退回貨物，必須有倉庫ID
						warehouseIDStr, _ := note["warehouse_id"].(string)
						if warehouseIDStr == "" {
							return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("取消採購單 %s 已選擇退回貨物，請選擇退回倉庫", cancellationNumber)})
						}

						shippingNoteIDStr, _ := note["shipping_note_id"].(string)
						hasExistingShippingNote := false
						if shippingNoteIDStr != "" {
							hasExistingShippingNote = true
						}

						// 即使已經有 shipping_note_id，也要補齊 shipping_note（用於前端顯示與重建 cancellation_shipping_notes）
						// 注意：此處只補資料，不做出庫扣庫存（扣庫存在 !hasExistingShippingNote 時才做）
						if _, exists := note["shipping_note"]; !exists {
							itemsRaw, _ := note["items"].([]interface{})
							shippingNumber := shippingNoteIDStr
							if shippingNumber == "" {
								shippingNumber = fmt.Sprintf("SHIP-CANC-%s", cancellationNumber)
								note["shipping_note_id"] = shippingNumber
							}

							itemsForShipping := make([]interface{}, 0)
							for _, ir := range itemsRaw {
								im, ok := ir.(map[string]interface{})
								if !ok {
									continue
								}
								pid, _ := im["product_id"].(string)
								if pid == "" {
									continue
								}
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
								qtyInt := int(qty)
								if qtyInt <= 0 {
									continue
								}
								itemsForShipping = append(itemsForShipping, map[string]interface{}{
									"product_id": pid,
									"quantity":   float64(qtyInt),
								})
							}

							shippingNote := map[string]interface{}{
								"shipping_number": shippingNumber,
								"shipping_date":   cancellationDate.Format("2006-01-02"),
								"warehouse_id":    warehouseIDStr,
								"notes":           fmt.Sprintf("採購單 %s 取消採購退回貨物", purchaseOrder.OrderNumber),
								"items":           itemsForShipping,
							}
							note["shipping_note"] = shippingNote
						}

						// 如果沒有現有的出貨記錄，且有產品明細，則創建新的
						if !hasExistingShippingNote {
							itemsRaw, _ := note["items"].([]interface{})
							if len(itemsRaw) > 0 {
								shippingNumber := fmt.Sprintf("SHIP-CANC-%s", cancellationNumber)

								shippingNote := map[string]interface{}{
									"shipping_number": shippingNumber,
									"shipping_date":   cancellationDate.Format("2006-01-02"),
									"warehouse_id":    warehouseIDStr,
									"notes":           fmt.Sprintf("採購單 %s 取消採購退回貨物", purchaseOrder.OrderNumber),
									"items":           []interface{}{},
								}

								// 提取產品明細
								itemsForShipping := make([]interface{}, 0)
								for _, ir := range itemsRaw {
									im, ok := ir.(map[string]interface{})
									if !ok {
										continue
									}
									pid, _ := im["product_id"].(string)
									if pid == "" {
										continue
									}
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
									qtyInt := int(qty)
									if qtyInt <= 0 {
										continue
									}
									itemsForShipping = append(itemsForShipping, map[string]interface{}{
										"product_id": pid,
										"quantity":   float64(qtyInt),
									})
								}
								shippingNote["items"] = itemsForShipping
								note["shipping_note"] = shippingNote
								note["shipping_note_id"] = shippingNumber

								// 直接出庫（減少庫存）
								whID, err := uuid.Parse(warehouseIDStr)
								if err != nil {
									log.Printf("Invalid warehouse_id in cancellation note: %v", err)
									continue
								}
								if whID != uuid.Nil {
									for _, itAny := range itemsForShipping {
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

										// 出庫（減少庫存）
										if whID != uuid.Nil && qtyInt > 0 {
											if err := utils.UpdateWarehouseStock(tenantID, pid, whID, qtyInt, "decrease"); err != nil {
												log.Printf("Failed to update warehouse stock for cancellation return: %v", err)
											}
										}
									}
								}
							}
						}
					}
				}

				// 保存 cancellation_notes 到 extra_fields
				fields["cancellation_notes"] = notesList
				// 同時保存出貨記錄到 cancellation_shipping_notes
				shippingNotes := make([]interface{}, 0)
				for _, noteInterface := range notesList {
					note, ok := noteInterface.(map[string]interface{})
					if !ok {
						continue
					}
					if shippingNote, exists := note["shipping_note"]; exists {
						shippingNotes = append(shippingNotes, shippingNote)
					}
				}
				if len(shippingNotes) > 0 {
					fields["cancellation_shipping_notes"] = shippingNotes
				}

				// 更新 extra_fields
				if purchaseOrder.ExtraFields == nil {
					purchaseOrder.ExtraFields = models.JSONB(make(map[string]interface{}))
				}
				currentExtraFields := map[string]interface{}(purchaseOrder.ExtraFields)
				for k, v := range fields {
					currentExtraFields[k] = v
				}
				purchaseOrder.ExtraFields = models.JSONB(currentExtraFields)
				database.DB.Model(&purchaseOrder).Update("extra_fields", purchaseOrder.ExtraFields)
			}
		}
	}

	return c.JSON(purchaseOrder)
}

func DeletePurchaseOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.PurchaseOrder{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Purchase order deleted"})
}

// ============================================
// 採購單明細 (PurchaseOrderItem) CRUD
// ============================================

func GetPurchaseOrderItems(c *fiber.Ctx) error {
	purchaseOrderID := c.Query("purchase_order_id")

	var items []models.PurchaseOrderItem
	query := database.DB

	if purchaseOrderID != "" {
		query = query.Where("purchase_order_id = ?", purchaseOrderID)
	}

	if err := query.Preload("PurchaseOrder").Preload("Product").Find(&items).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": items})
}

func GetPurchaseOrderItem(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.PurchaseOrderItem

	if err := database.DB.Preload("PurchaseOrder").Preload("Product").First(&item, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order item not found"})
	}

	return c.JSON(item)
}

func CreatePurchaseOrderItem(c *fiber.Ctx) error {
	var item models.PurchaseOrderItem
	if err := c.BodyParser(&item); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 計算總金額
	item.TotalAmount = float64(item.Quantity)*item.UnitPrice - item.DiscountAmount

	if err := database.DB.Create(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新採購單總金額
	updatePurchaseOrderTotal(item.PurchaseOrderID)

	return c.Status(201).JSON(item)
}

func UpdatePurchaseOrderItem(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.PurchaseOrderItem

	if err := database.DB.First(&item, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order item not found"})
	}

	oldPurchaseOrderID := item.PurchaseOrderID

	if err := c.BodyParser(&item); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 計算總金額
	item.TotalAmount = float64(item.Quantity)*item.UnitPrice - item.DiscountAmount

	if err := database.DB.Save(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新採購單總金額
	updatePurchaseOrderTotal(item.PurchaseOrderID)
	if oldPurchaseOrderID != item.PurchaseOrderID {
		updatePurchaseOrderTotal(oldPurchaseOrderID)
	}

	return c.JSON(item)
}

func DeletePurchaseOrderItem(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.PurchaseOrderItem

	if err := database.DB.First(&item, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order item not found"})
	}

	purchaseOrderID := item.PurchaseOrderID

	if err := database.DB.Delete(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新採購單總金額
	updatePurchaseOrderTotal(purchaseOrderID)

	return c.JSON(fiber.Map{"message": "Purchase order item deleted"})
}

// updatePurchaseOrderTotal 更新採購單總金額
func updatePurchaseOrderTotal(purchaseOrderID uuid.UUID) {
	var total float64
	database.DB.Model(&models.PurchaseOrderItem{}).
		Where("purchase_order_id = ?", purchaseOrderID).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&total)

	database.DB.Model(&models.PurchaseOrder{}).
		Where("id = ?", purchaseOrderID).
		Updates(map[string]interface{}{
			"total_amount": total,
			"final_amount": total,
		})
}
