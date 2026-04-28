package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// GetInvoices 獲取發票列表
func GetInvoices(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var invoices []models.Invoice
	query := database.DB.Where("tenant_id = ?", tenantID).Preload("Customer").Preload("Order")

	// 搜索過濾
	if search := c.Query("search"); search != "" {
		query = query.Where("invoice_number ILIKE ?", "%"+search+"%")
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Invoice{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&invoices).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch invoices"})
	}

	return c.JSON(fiber.Map{
		"data":  invoices,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetInvoice 獲取單個發票
func GetInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid invoice ID"})
	}

	var invoice models.Invoice
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("Customer").Preload("Order").Preload("Payments").First(&invoice).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Invoice not found"})
	}

	return c.JSON(invoice)
}

// CreateInvoice 創建發票
func CreateInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		InvoiceNumber string                 `json:"invoice_number"`
		OrderID       *uuid.UUID             `json:"order_id"`
		CustomerID    *uuid.UUID             `json:"customer_id"`
		InvoiceDate   string                 `json:"invoice_date"`
		DueDate       *string                `json:"due_date"`
		Status        string                 `json:"status"`
		Subtotal      float64                `json:"subtotal"`
		TaxAmount     float64                `json:"tax_amount"`
		Notes         string                 `json:"notes"`
		ExtraFields   map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 如果沒有提供發票號，自動生成
	if req.InvoiceNumber == "" {
		req.InvoiceNumber = "INV-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
	}

	// 檢查發票號是否已存在
	var existingInvoice models.Invoice
	if err := database.DB.Where("tenant_id = ? AND invoice_number = ?", tenantID, req.InvoiceNumber).First(&existingInvoice).Error; err == nil {
		// 如果已存在，重新生成
		req.InvoiceNumber = "INV-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
	}

	// 解析日期（使用租戶時區）
	invoiceDate, err := utils.ParseDateInTenantTimezone(tenantID, req.InvoiceDate)
	if err != nil {
		invoiceDate = utils.NowInTenantTimezone(tenantID)
	}

	var dueDate *time.Time
	if req.DueDate != nil {
		if d, err := utils.ParseDateInTenantTimezone(tenantID, *req.DueDate); err == nil {
			dueDate = &d
		}
	}

	if req.Status == "" {
		req.Status = "draft"
	}

	totalAmount := req.Subtotal + req.TaxAmount

	now := utils.NowInTenantTimezone(tenantID)
	invoice := models.Invoice{
		TenantID:      tenantID,
		InvoiceNumber: req.InvoiceNumber,
		OrderID:       req.OrderID,
		CustomerID:    req.CustomerID,
		InvoiceDate:   invoiceDate,
		DueDate:       dueDate,
		Status:        req.Status,
		Subtotal:      req.Subtotal,
		TaxAmount:     req.TaxAmount,
		TotalAmount:   totalAmount,
		PaidAmount:    0,
		Notes:         req.Notes,
		CreatedBy:     &userID,
		UpdatedBy:     &userID,
		CreatedAt:     now,
		UpdatedAt:     now,
		ExtraFields:   models.JSONB(req.ExtraFields),
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&invoice).Error; err != nil {
			return err
		}
		if err := syncInvoiceJournalEntryTx(tx, &invoice, &userID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create invoice"})
	}

	database.DB.Where("id = ?", invoice.ID).Preload("Customer").Preload("Order").First(&invoice)

	return c.Status(201).JSON(invoice)
}

// UpdateInvoice 更新發票
func UpdateInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid invoice ID"})
	}

	var invoice models.Invoice
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&invoice).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Invoice not found"})
	}

	var req struct {
		CustomerID  *uuid.UUID              `json:"customer_id"`
		OrderID     *uuid.UUID              `json:"order_id"`
		InvoiceDate *string                 `json:"invoice_date"`
		DueDate     *string                 `json:"due_date"`
		Status      *string                 `json:"status"`
		Subtotal    *float64                `json:"subtotal"`
		TaxAmount   *float64                `json:"tax_amount"`
		TotalAmount *float64                `json:"total_amount"`
		PaidAmount  *float64                `json:"paid_amount"`
		Notes       *string                 `json:"notes"`
		ExtraFields *map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.CustomerID != nil {
		invoice.CustomerID = req.CustomerID
	}
	if req.OrderID != nil {
		invoice.OrderID = req.OrderID
	}
	if req.InvoiceDate != nil {
		if date, err := utils.ParseDateInTenantTimezone(tenantID, *req.InvoiceDate); err == nil {
			invoice.InvoiceDate = date
		}
	}
	if req.DueDate != nil {
		if d, err := utils.ParseDateInTenantTimezone(tenantID, *req.DueDate); err == nil {
			invoice.DueDate = &d
		}
	}
	if req.Status != nil {
		invoice.Status = *req.Status
	}
	if req.Subtotal != nil {
		invoice.Subtotal = *req.Subtotal
	}
	if req.TaxAmount != nil {
		invoice.TaxAmount = *req.TaxAmount
	}
	if req.TotalAmount != nil {
		invoice.TotalAmount = *req.TotalAmount
	} else if req.Subtotal != nil && req.TaxAmount != nil {
		// 自動計算總金額
		invoice.TotalAmount = *req.Subtotal + *req.TaxAmount
	}
	if req.PaidAmount != nil {
		invoice.PaidAmount = *req.PaidAmount
	}
	if req.Notes != nil {
		invoice.Notes = *req.Notes
	}
	if req.ExtraFields != nil {
		invoice.ExtraFields = models.JSONB(*req.ExtraFields)
	}

	invoice.UpdatedBy = &userID
	invoice.UpdatedAt = time.Now()

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&invoice).Error; err != nil {
			return err
		}
		if err := syncInvoiceJournalEntryTx(tx, &invoice, &userID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update invoice"})
	}

	database.DB.Where("id = ?", invoice.ID).Preload("Customer").Preload("Order").First(&invoice)

	return c.JSON(invoice)
}

// DeleteInvoice 刪除發票
func DeleteInvoice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid invoice ID"})
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := removeJournalEntryForSource(tx, tenantID, "invoice", id); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.Invoice{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete invoice"})
	}

	return c.JSON(fiber.Map{"message": "Invoice deleted successfully"})
}

// CreatePayment 創建支付記錄
func CreatePayment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		InvoiceID       *uuid.UUID             `json:"invoice_id"`
		PaymentDate     string                 `json:"payment_date"`
		Amount          float64                `json:"amount"`
		PaymentMethod   string                 `json:"payment_method"`
		ReferenceNumber string                 `json:"reference_number"`
		Notes           string                 `json:"notes"`
		ExtraFields     map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Amount <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Amount must be greater than 0"})
	}

	// 解析日期（使用租戶時區）
	paymentDate, err := utils.ParseDateInTenantTimezone(tenantID, req.PaymentDate)
	if err != nil {
		paymentDate = utils.NowInTenantTimezone(tenantID)
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	payment := models.Payment{
		TenantID:        tenantID,
		InvoiceID:       req.InvoiceID,
		PaymentDate:     paymentDate,
		Amount:          req.Amount,
		PaymentMethod:   req.PaymentMethod,
		ReferenceNumber: req.ReferenceNumber,
		Notes:           req.Notes,
		CreatedBy:       &userID,
		CreatedAt:       time.Now(),
		ExtraFields:     models.JSONB(req.ExtraFields),
	}

	if err := tx.Create(&payment).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create payment"})
	}

	// 如果關聯發票，更新發票的已付金額
	if req.InvoiceID != nil {
		var invoice models.Invoice
		if err := tx.Where("id = ? AND tenant_id = ?", *req.InvoiceID, tenantID).First(&invoice).Error; err == nil {
			invoice.PaidAmount += req.Amount
			if invoice.PaidAmount >= invoice.TotalAmount {
				invoice.Status = "paid"
			}
			if err := tx.Save(&invoice).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to update invoice"})
			}
		}
	}

	tx.Commit()

	database.DB.Where("id = ?", payment.ID).Preload("Invoice").First(&payment)

	return c.Status(201).JSON(payment)
}

// GetPayments 獲取支付記錄列表
func GetPayments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var payments []models.Payment
	query := database.DB.Where("tenant_id = ?", tenantID).Preload("Invoice")

	// 發票過濾
	if invoiceID := c.Query("invoice_id"); invoiceID != "" {
		if id, err := uuid.Parse(invoiceID); err == nil {
			query = query.Where("invoice_id = ?", id)
		}
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Payment{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&payments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch payments"})
	}

	return c.JSON(fiber.Map{
		"data":  payments,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// ExportInvoicesToExcel 導出發票到 Excel
func ExportInvoicesToExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var invoices []models.Invoice
	query := database.DB.Where("tenant_id = ?", tenantID).Preload("Customer")

	if search := c.Query("search"); search != "" {
		query = query.Where("invoice_number ILIKE ?", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Order("created_at DESC").Find(&invoices).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch invoices"})
	}

	f := excelize.NewFile()
	defer f.Close()
	sheetName := "發票列表"
	f.NewSheet(sheetName)
	f.DeleteSheet("Sheet1")

	headers := []string{"發票號", "客戶", "日期", "總金額", "已付", "未付", "狀態"}
	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheetName, cell, header)
		style, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
		})
		f.SetCellStyle(sheetName, cell, cell, style)
	}

	for i, invoice := range invoices {
		row := i + 2
		customerName := "無客戶"
		if invoice.Customer != nil {
			customerName = invoice.Customer.Name
		}
		unpaid := invoice.TotalAmount - invoice.PaidAmount
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), invoice.InvoiceNumber)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), customerName)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), invoice.InvoiceDate.Format("2006-01-02"))
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), invoice.TotalAmount)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), invoice.PaidAmount)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), unpaid)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), invoice.Status)
	}

	for i := 0; i < len(headers); i++ {
		f.SetColWidth(sheetName, string(rune('A'+i)), string(rune('A'+i)), 18)
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename=invoices.xlsx")
	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate Excel file"})
	}
	return nil
}

// ExportInvoicesToPDF 導出發票到 PDF
func ExportInvoicesToPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var invoices []models.Invoice
	query := database.DB.Where("tenant_id = ?", tenantID).Preload("Customer")

	if search := c.Query("search"); search != "" {
		query = query.Where("invoice_number ILIKE ?", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Order("created_at DESC").Find(&invoices).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch invoices"})
	}

	headers := []string{"發票號", "客戶", "日期", "總金額", "已付", "未付", "狀態"}
	rows := make([][]string, 0, len(invoices))
	for _, invoice := range invoices {
		customerName := "無客戶"
		if invoice.Customer != nil {
			customerName = invoice.Customer.Name
		}
		unpaid := invoice.TotalAmount - invoice.PaidAmount
		rows = append(rows, []string{
			invoice.InvoiceNumber,
			customerName,
			invoice.InvoiceDate.Format("2006-01-02"),
			fmt.Sprintf("%.2f", invoice.TotalAmount),
			fmt.Sprintf("%.2f", invoice.PaidAmount),
			fmt.Sprintf("%.2f", unpaid),
			invoice.Status,
		})
	}
	pdfBytes, _ := utils.BuildTablePDFBytes("發票列表", headers, rows)
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", "attachment; filename=invoices.pdf")
	return c.Send(pdfBytes)
}

// ---------------------------------------------------------------------------
// syncIncomeToInvoice 在建立 Income（付款記錄）後，自動同步到 invoices 表。
// 此函式是 fire-and-forget：失敗不影響原本的 Income 建立流程。
// ---------------------------------------------------------------------------
func syncIncomeToInvoice(db *gorm.DB, income *models.Income) {
	if income == nil || income.ReferenceID == nil {
		return
	}
	if income.ReferenceType != "order" && income.ReferenceType != "service_order" {
		return
	}
	syncOrderInvoice(db, income.TenantID, income.ReferenceType, *income.ReferenceID, income.CreatedBy)
}

// syncOrderInvoice 同步訂單/服務單的 Invoice 狀態。
//
// 邏輯：
//  1. 只處理 referenceType = "order" 或 "service_order"。
//  2. 以「一個訂單/服務單 → 一張 Invoice」為原則，用 order_id 查找已有 Invoice。
//  3. 若不存在且有付款記錄，自動建立一張 Invoice（狀態 sent）。
//  4. 彙總該訂單所有 Income 的金額，更新 Invoice.PaidAmount 和 Status。
//  5. 若所有付款記錄被刪除（totalPaid = 0），保留 Invoice 但狀態回到 sent。
//
// 此函式是 fire-and-forget：失敗不影響原本的業務流程。
func syncOrderInvoice(db *gorm.DB, tenantID uuid.UUID, referenceType string, orderID uuid.UUID, userID *uuid.UUID) {
	if referenceType != "order" && referenceType != "service_order" {
		return
	}

	// 取得關聯的訂單/服務單資料
	var customerID *uuid.UUID
	var totalAmount float64
	var orderDate time.Time
	var orderNumber string

	switch referenceType {
	case "order":
		var order models.Order
		if err := db.Select("id, customer_id, total_amount, order_date, order_number").
			Where("tenant_id = ? AND id = ?", tenantID, orderID).
			First(&order).Error; err != nil {
			return
		}
		customerID = order.CustomerID
		totalAmount = order.TotalAmount
		orderDate = order.OrderDate
		orderNumber = order.OrderNumber
	case "service_order":
		var so models.ServiceOrder
		if err := db.Select("id, customer_id, total_amount, service_date, order_number").
			Where("tenant_id = ? AND id = ?", tenantID, orderID).
			First(&so).Error; err != nil {
			return
		}
		customerID = so.CustomerID
		totalAmount = so.TotalAmount
		orderDate = so.ServiceDate
		orderNumber = so.OrderNumber
	}

	// 彙總該訂單所有已確認 Income 的付款金額
	var totalPaid float64
	db.Model(&models.Income{}).
		Where("tenant_id = ? AND reference_type = ? AND reference_id = ? AND status = ?",
			tenantID, referenceType, orderID, "confirmed").
		Select("COALESCE(SUM(amount), 0)").Scan(&totalPaid)

	// 取得第一筆 income 的發票號碼（用於建立 Invoice 時）
	var firstInvoiceNumber string
	var firstIncome models.Income
	if err := db.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?",
		tenantID, referenceType, orderID).
		Order("created_at ASC").First(&firstIncome).Error; err == nil {
		if firstIncome.ExtraFields != nil {
			if n, ok := firstIncome.ExtraFields["invoice_number"].(string); ok {
				firstInvoiceNumber = n
			}
		}
	}

	// 查找或建立 Invoice
	var invoice models.Invoice
	err := db.Where("tenant_id = ? AND order_id = ?", tenantID, orderID).First(&invoice).Error
	if err != nil {
		// 不存在 → 只在有付款記錄時才建立
		if totalPaid <= 0 && firstInvoiceNumber == "" {
			return // 沒有付款記錄也沒有 Invoice，不建立空 Invoice
		}
		invNum := firstInvoiceNumber
		if invNum == "" {
			invNum = fmt.Sprintf("INV-%s", orderNumber)
		}
		invoice = models.Invoice{
			TenantID:      tenantID,
			InvoiceNumber: invNum,
			OrderID:       &orderID,
			CustomerID:    customerID,
			InvoiceDate:   orderDate,
			Status:        "sent",
			Subtotal:      totalAmount,
			TaxAmount:     0,
			TotalAmount:   totalAmount,
			PaidAmount:    0,
			CreatedBy:     userID,
			UpdatedBy:     userID,
		}
		if err := db.Create(&invoice).Error; err != nil {
			return
		}
	}

	// 更新 Invoice 的 PaidAmount 和 Status
	newStatus := invoice.Status
	if totalPaid >= totalAmount && totalAmount > 0 {
		newStatus = "paid"
	} else if totalPaid > 0 {
		newStatus = "sent"
	} else {
		// totalPaid == 0，所有付款被刪除
		if invoice.Status == "paid" {
			newStatus = "sent"
		}
	}
	db.Model(&invoice).Updates(map[string]interface{}{
		"paid_amount":  totalPaid,
		"status":       newStatus,
		"total_amount": totalAmount,
		"subtotal":     totalAmount,
		"customer_id":  customerID,
	})
}
