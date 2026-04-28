package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func appendRefToDescription(desc string, ref string) string {
	desc = strings.TrimSpace(desc)
	ref = strings.TrimSpace(ref)
	if desc == "" || ref == "" {
		return desc
	}
	// avoid double append
	if strings.Contains(desc, "("+ref+")") || strings.HasSuffix(desc, " "+ref) {
		return desc
	}
	return fmt.Sprintf("%s (%s)", desc, ref)
}

// ============================================
// 收入 (Income) CRUD
// ============================================

func GetIncomes(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var incomes []models.Income
	query := database.DB.Where("tenant_id = ?", tenantID)

	// 篩選條件
	if incomeType := c.Query("income_type"); incomeType != "" {
		query = query.Where("income_type = ?", incomeType)
	}
	if refType := c.Query("reference_type"); refType != "" {
		query = query.Where("reference_type = ?", refType)
	}
	if refID := c.Query("reference_id"); refID != "" {
		if parsed, err := uuid.Parse(refID); err == nil {
			query = query.Where("reference_id = ?", parsed)
		}
	}
	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if startDate := c.Query("start_date"); startDate != "" {
		query = query.Where("income_date >= ?", startDate)
	}
	if endDate := c.Query("end_date"); endDate != "" {
		query = query.Where("income_date <= ?", endDate)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Income{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("income_date DESC, created_at DESC").
		Preload("BankAccount").
		Find(&incomes).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 手動加載關聯的訂單/服務單（因為沒有外鍵關係，需要根據 reference_id 和 reference_type 加載）
	for i := range incomes {
		if incomes[i].ReferenceID != nil {
			if (incomes[i].ReferenceType == "order" || incomes[i].Category == "order") && incomes[i].Order == nil {
				var order models.Order
				if err := database.DB.Where("id = ? AND tenant_id = ?", incomes[i].ReferenceID, tenantID).
					Select("id, order_number").First(&order).Error; err == nil {
					incomes[i].Order = &order
				}
			} else if (incomes[i].ReferenceType == "service_order" || incomes[i].Category == "service_order") && incomes[i].ServiceOrder == nil {
				var serviceOrder models.ServiceOrder
				if err := database.DB.Where("id = ? AND tenant_id = ?", incomes[i].ReferenceID, tenantID).
					Select("id, order_number").First(&serviceOrder).Error; err == nil {
					incomes[i].ServiceOrder = &serviceOrder
				}
			}
		}
		// 描述輸出：描述 + (關聯單號)
		if incomes[i].Order != nil && incomes[i].Order.OrderNumber != "" {
			incomes[i].Description = appendRefToDescription(incomes[i].Description, incomes[i].Order.OrderNumber)
		} else if incomes[i].ServiceOrder != nil && incomes[i].ServiceOrder.OrderNumber != "" {
			incomes[i].Description = appendRefToDescription(incomes[i].Description, incomes[i].ServiceOrder.OrderNumber)
		}
	}

	return c.JSON(fiber.Map{
		"data":  incomes,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetIncome(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid income ID"})
	}

	var income models.Income
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("BankAccount").
		First(&income).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Income not found"})
	}

	// 從 ExtraFields 中提取 payment_method_id 和 reference_number
	result := make(map[string]interface{})
	result["id"] = income.ID
	result["tenant_id"] = income.TenantID
	result["income_type"] = income.IncomeType
	result["reference_id"] = income.ReferenceID
	result["reference_type"] = income.ReferenceType
	result["category"] = income.Category
	result["description"] = income.Description
	result["amount"] = income.Amount
	result["income_date"] = income.IncomeDate
	result["payment_method"] = income.PaymentMethod
	result["bank_account_id"] = income.BankAccountID
	result["bank_account"] = income.BankAccount
	result["status"] = income.Status
	result["notes"] = income.Notes
	result["created_by"] = income.CreatedBy
	result["updated_by"] = income.UpdatedBy
	result["created_at"] = income.CreatedAt
	result["updated_at"] = income.UpdatedAt

	// 從 ExtraFields 中提取 payment_method_id 和 reference_number
	if income.ExtraFields != nil {
		fields := map[string]interface{}(income.ExtraFields)
		if paymentMethodID, exists := fields["payment_method_id"]; exists {
			result["payment_method_id"] = paymentMethodID
		}
		if referenceNumber, exists := fields["reference_number"]; exists {
			result["reference_number"] = referenceNumber
		}
		if invoiceNumber, exists := fields["invoice_number"]; exists {
			result["invoice_number"] = invoiceNumber
		}
	}

	return c.JSON(result)
}

func CreateIncome(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		RelatedUserID *uuid.UUID             `json:"related_user_id"`
		IncomeType    string                 `json:"income_type"`
		ReferenceID   *uuid.UUID             `json:"reference_id"`
		ReferenceType string                 `json:"reference_type"`
		Category      string                 `json:"category"`
		Description   string                 `json:"description"`
		Amount        float64                `json:"amount"`
		IncomeDate    string                 `json:"income_date"`
		PaymentMethod string                 `json:"payment_method"`
		BankAccountID *uuid.UUID             `json:"bank_account_id"`
		Status        string                 `json:"status"`
		Notes         string                 `json:"notes"`
		ExtraFields   map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 根據類別設置 reference_type
	if req.ReferenceType == "" {
		if req.Category == "order" {
			req.ReferenceType = "order"
		} else if req.Category == "service_order" {
			req.ReferenceType = "service_order"
		}
	}

	// 設置 income_type（如果沒有提供，使用 category）
	if req.IncomeType == "" {
		req.IncomeType = req.Category
		if req.IncomeType == "" {
			req.IncomeType = "other"
		}
	}

	// 預設相關人員：
	// - 若前端未提供 related_user_id，則優先從關聯單據帶入銷售員
	// - 若仍沒有（例如沒有銷售員），則預設為目前登入者
	if req.RelatedUserID == nil {
		if req.ReferenceID != nil && req.ReferenceType != "" {
			switch req.ReferenceType {
			case "order":
				var order models.Order
				if err := database.DB.
					Select("salesperson_id").
					Where("tenant_id = ? AND id = ?", tenantID, *req.ReferenceID).
					First(&order).Error; err == nil {
					if order.SalespersonID != nil {
						req.RelatedUserID = order.SalespersonID
					}
				}
			case "service_order":
				var so models.ServiceOrder
				if err := database.DB.
					Select("salesperson_id").
					Where("tenant_id = ? AND id = ?", tenantID, *req.ReferenceID).
					First(&so).Error; err == nil {
					if so.SalespersonID != nil {
						req.RelatedUserID = so.SalespersonID
					}
				}
			}
		}
		if req.RelatedUserID == nil {
			req.RelatedUserID = &userID
		}
	}

	// 解析日期（使用租戶時區）
	incomeDate, err := utils.ParseDateInTenantTimezone(tenantID, req.IncomeDate)
	if err != nil {
		incomeDate = utils.NowInTenantTimezone(tenantID)
	}

	if req.Status == "" {
		req.Status = "confirmed"
	}

	now := utils.NowInTenantTimezone(tenantID)

	// 構建 ExtraFields，包含 payment_method_id 和 reference_number
	extraFields := make(map[string]interface{})
	if req.ExtraFields != nil {
		extraFields = req.ExtraFields
	}
	// payment_method_id 和 reference_number 應該從 ExtraFields 中獲取（前端會通過 extra_fields 傳遞）

	income := models.Income{
		TenantID:      tenantID,
		RelatedUserID: req.RelatedUserID,
		IncomeType:    req.IncomeType,
		ReferenceID:   req.ReferenceID,
		ReferenceType: req.ReferenceType,
		Category:      req.Category,
		Description:   req.Description,
		Amount:        req.Amount,
		IncomeDate:    incomeDate,
		PaymentMethod: req.PaymentMethod,
		BankAccountID: req.BankAccountID,
		Status:        req.Status,
		Notes:         req.Notes,
		CreatedBy:     &userID,
		UpdatedBy:     &userID,
		CreatedAt:     now,
		UpdatedAt:     now,
		ExtraFields:   models.JSONB(extraFields),
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&income).Error; err != nil {
			return err
		}
		if err := syncIncomeJournalEntryTx(tx, &income, &userID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create income"})
	}

	// 同步 Invoice（transaction 已 commit，Income 資料已持久化）
	syncIncomeToInvoice(database.DB, &income)

	return c.Status(201).JSON(income)
}

func UpdateIncome(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid income ID"})
	}

	var income models.Income
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&income).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Income not found"})
	}

	var req struct {
		RelatedUserID   *uuid.UUID             `json:"related_user_id"`
		IncomeType      string                 `json:"income_type"`
		ReferenceID     *uuid.UUID             `json:"reference_id"`
		ReferenceType   string                 `json:"reference_type"`
		Category        string                 `json:"category"`
		Description     string                 `json:"description"`
		Amount          float64                `json:"amount"`
		IncomeDate      string                 `json:"income_date"`
		PaymentMethod   string                 `json:"payment_method"`
		PaymentMethodID *uuid.UUID             `json:"payment_method_id"`
		ReferenceNumber string                 `json:"reference_number"`
		BankAccountID   *uuid.UUID             `json:"bank_account_id"`
		Status          string                 `json:"status"`
		Notes           string                 `json:"notes"`
		ExtraFields     map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 根據類別設置 reference_type
	if req.ReferenceType == "" {
		if req.Category == "order" {
			req.ReferenceType = "order"
		} else if req.Category == "service_order" {
			req.ReferenceType = "service_order"
		}
	}

	// 設置 income_type（如果沒有提供，使用 category）
	if req.IncomeType == "" {
		req.IncomeType = req.Category
		if req.IncomeType == "" {
			req.IncomeType = "other"
		}
	}

	// 更新字段
	income.RelatedUserID = req.RelatedUserID
	income.IncomeType = req.IncomeType
	income.ReferenceID = req.ReferenceID
	income.ReferenceType = req.ReferenceType
	income.Category = req.Category
	income.Description = req.Description
	income.Amount = req.Amount
	income.PaymentMethod = req.PaymentMethod
	income.BankAccountID = req.BankAccountID
	income.Status = req.Status
	income.Notes = req.Notes
	income.UpdatedBy = &userID
	income.UpdatedAt = time.Now()

	if req.IncomeDate != "" {
		if incomeDate, err := utils.ParseDateInTenantTimezone(tenantID, req.IncomeDate); err == nil {
			income.IncomeDate = incomeDate
		}
	}

	// 構建 ExtraFields，包含 payment_method_id 和 reference_number
	extraFields := make(map[string]interface{})
	if req.ExtraFields != nil {
		extraFields = req.ExtraFields
	}
	if req.PaymentMethodID != nil {
		extraFields["payment_method_id"] = req.PaymentMethodID.String()
	}
	if req.ReferenceNumber != "" {
		extraFields["reference_number"] = req.ReferenceNumber
	}
	if len(extraFields) > 0 {
		income.ExtraFields = models.JSONB(extraFields)
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&income).Error; err != nil {
			return err
		}
		if err := syncIncomeJournalEntryTx(tx, &income, &userID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update income"})
	}

	// 同步 Invoice（金額或狀態可能已變更）
	syncIncomeToInvoice(database.DB, &income)

	return c.JSON(income)
}

func DeleteIncome(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid income ID"})
	}

	var income models.Income
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&income).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Income not found"})
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := removeJournalEntryForSource(tx, tenantID, "income", income.ID); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.Income{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete income"})
	}

	// 同步 Invoice（Income 已刪除，重新計算 PaidAmount）
	if income.ReferenceID != nil {
		syncOrderInvoice(database.DB, tenantID, income.ReferenceType, *income.ReferenceID, nil)
	}

	return c.JSON(fiber.Map{"message": "Income deleted successfully"})
}

// ============================================
// 支出 (Expense) CRUD
// ============================================

func GetExpenses(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var expenses []models.Expense
	query := database.DB.Where("tenant_id = ?", tenantID)

	// 篩選條件
	if projectID := c.Query("project_id"); projectID != "" {
		if pid, err := uuid.Parse(projectID); err == nil {
			query = query.Where("project_id = ?", pid)
		}
	}
	if expenseType := c.Query("expense_type"); expenseType != "" {
		query = query.Where("expense_type = ?", expenseType)
	}
	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if startDate := c.Query("start_date"); startDate != "" {
		query = query.Where("expense_date >= ?", startDate)
	}
	if endDate := c.Query("end_date"); endDate != "" {
		query = query.Where("expense_date <= ?", endDate)
	}
	if referenceType := c.Query("reference_type"); referenceType != "" {
		query = query.Where("reference_type = ?", referenceType)
	}
	if referenceID := c.Query("reference_id"); referenceID != "" {
		if refID, err := uuid.Parse(referenceID); err == nil {
			query = query.Where("reference_id = ?", refID)
		}
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Expense{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("expense_date DESC, created_at DESC").
		Preload("BankAccount").
		Find(&expenses).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 手動加載關聯的訂單/服務單/採購單（因為沒有外鍵關係，需要根據 reference_id 和 reference_type/category 加載）
	for i := range expenses {
		if expenses[i].ReferenceID != nil {
			if (expenses[i].ReferenceType == "purchase_order" || expenses[i].Category == "purchase") && expenses[i].PurchaseOrder == nil {
				var purchaseOrder models.PurchaseOrder
				if err := database.DB.Where("id = ? AND tenant_id = ?", expenses[i].ReferenceID, tenantID).
					Select("id, order_number").First(&purchaseOrder).Error; err == nil {
					expenses[i].PurchaseOrder = &purchaseOrder
				}
			} else if (expenses[i].ReferenceType == "order" || expenses[i].Category == "order_commission" || expenses[i].Category == "product_tax" || expenses[i].Category == "refund") && expenses[i].Order == nil {
				var order models.Order
				if err := database.DB.Where("id = ? AND tenant_id = ?", expenses[i].ReferenceID, tenantID).
					Select("id, order_number").First(&order).Error; err == nil {
					expenses[i].Order = &order
				}
			} else if (expenses[i].ReferenceType == "service_order" || expenses[i].Category == "service_order_commission" || expenses[i].Category == "service_tax") && expenses[i].ServiceOrder == nil {
				var serviceOrder models.ServiceOrder
				if err := database.DB.Where("id = ? AND tenant_id = ?", expenses[i].ReferenceID, tenantID).
					Select("id, order_number").First(&serviceOrder).Error; err == nil {
					expenses[i].ServiceOrder = &serviceOrder
				}
			}
		}
		// 描述輸出：描述 + (關聯單號)
		if expenses[i].PurchaseOrder != nil && expenses[i].PurchaseOrder.OrderNumber != "" {
			expenses[i].Description = appendRefToDescription(expenses[i].Description, expenses[i].PurchaseOrder.OrderNumber)
		} else if expenses[i].Order != nil && expenses[i].Order.OrderNumber != "" {
			expenses[i].Description = appendRefToDescription(expenses[i].Description, expenses[i].Order.OrderNumber)
		} else if expenses[i].ServiceOrder != nil && expenses[i].ServiceOrder.OrderNumber != "" {
			expenses[i].Description = appendRefToDescription(expenses[i].Description, expenses[i].ServiceOrder.OrderNumber)
		}
	}

	return c.JSON(fiber.Map{
		"data":  expenses,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetExpense(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid expense ID"})
	}

	var expense models.Expense
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&expense).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Expense not found"})
	}

	return c.JSON(expense)
}

func CreateExpense(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		ProjectID     *uuid.UUID             `json:"project_id"`
		RelatedUserID *uuid.UUID             `json:"related_user_id"`
		ExpenseType   string                 `json:"expense_type"`
		ReferenceID   *uuid.UUID             `json:"reference_id"`
		ReferenceType string                 `json:"reference_type"`
		Category      string                 `json:"category"`
		Description   string                 `json:"description"`
		Amount        float64                `json:"amount"`
		ExpenseDate   string                 `json:"expense_date"`
		PaymentMethod string                 `json:"payment_method"`
		BankAccountID *uuid.UUID             `json:"bank_account_id"`
		Vendor        string                 `json:"vendor"`
		Status        string                 `json:"status"`
		Notes         string                 `json:"notes"`
		ExtraFields   map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 預設相關人員：若未提供，預設為目前登入者
	if req.RelatedUserID == nil {
		req.RelatedUserID = &userID
	}

	// 設置 expense_type（如果沒有提供，使用 category）
	if req.ExpenseType == "" {
		req.ExpenseType = req.Category
		if req.ExpenseType == "" {
			req.ExpenseType = "other"
		}
	}

	// 根據類別設置 reference_type（例如 purchase -> purchase_order）
	if req.ReferenceType == "" {
		if req.Category == "purchase" {
			req.ReferenceType = "purchase_order"
		} else if req.Category == "project" {
			req.ReferenceType = "project"
		} else if req.Category == "refund" {
			req.ReferenceType = "order"
		}
	}

	// 解析日期（使用租戶時區）
	expenseDate, err := utils.ParseDateInTenantTimezone(tenantID, req.ExpenseDate)
	if err != nil {
		expenseDate = utils.NowInTenantTimezone(tenantID)
	}

	if req.Status == "" {
		req.Status = "confirmed"
	}

	now := utils.NowInTenantTimezone(tenantID)
	expense := models.Expense{
		TenantID:      tenantID,
		ProjectID:     req.ProjectID,
		RelatedUserID: req.RelatedUserID,
		ExpenseType:   req.ExpenseType,
		ReferenceID:   req.ReferenceID,
		ReferenceType: req.ReferenceType,
		Category:      req.Category,
		Description:   req.Description,
		Amount:        req.Amount,
		ExpenseDate:   expenseDate,
		PaymentMethod: req.PaymentMethod,
		BankAccountID: req.BankAccountID,
		Vendor:        req.Vendor,
		Status:        req.Status,
		Notes:         req.Notes,
		CreatedBy:     &userID,
		UpdatedBy:     &userID,
		CreatedAt:     now,
		UpdatedAt:     now,
		ExtraFields:   models.JSONB(req.ExtraFields),
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&expense).Error; err != nil {
			return err
		}
		if err := syncExpenseJournalEntryTx(tx, &expense, &userID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create expense"})
	}

	return c.Status(201).JSON(expense)
}

// GenerateMonthlyCommissions 自動生成本月所有訂單的佣金支出
func GenerateMonthlyCommissions(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	// 獲取本月開始和結束時間
	now := time.Now()
	year, month, _ := now.Date()
	startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

	// 查詢本月所有有佣金的訂單
	var orders []models.Order
	if err := database.DB.Where("tenant_id = ? AND salesperson_id IS NOT NULL AND order_date >= ? AND order_date <= ?",
		tenantID, startOfMonth, endOfMonth).
		Preload("Salesperson").
		Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch orders"})
	}

	createdCount := 0
	for _, order := range orders {
		if order.SalespersonID == nil || order.CommissionAmount <= 0 {
			// 如果沒有保存 commission_amount，嘗試用銷售員佣金設定即時計算（支援 % / fixed）
			if order.Salesperson == nil || order.SalespersonID == nil {
				continue
			}
			// percent: commission_rate（百分比）
			// fixed: extra_fields.commission_mode=fixed + extra_fields.commission_fixed
			mode := ""
			fixed := 0.0
			if order.Salesperson.ExtraFields != nil {
				fields := map[string]interface{}(order.Salesperson.ExtraFields)
				if v, ok := fields["commission_mode"].(string); ok {
					mode = v
				}
				switch v := fields["commission_fixed"].(type) {
				case float64:
					fixed = v
				case float32:
					fixed = float64(v)
				case int:
					fixed = float64(v)
				case int64:
					fixed = float64(v)
				case string:
					if vv, err := strconv.ParseFloat(v, 64); err == nil {
						fixed = vv
					}
				}
			}
			if mode == "fixed" {
				order.CommissionAmount = fixed
			} else if order.Salesperson.CommissionRate > 0 {
				order.CommissionAmount = order.TotalAmount * (order.Salesperson.CommissionRate / 100.0)
			}
			if order.CommissionAmount <= 0 {
				continue
			}
		}

		// 檢查是否已經存在該訂單的佣金支出
		var existingExpense models.Expense
		if err := database.DB.Where("tenant_id = ? AND category = ? AND extra_fields->>'order_id' = ?",
			tenantID, "commission", order.ID.String()).First(&existingExpense).Error; err == nil {
			// 已存在，跳過
			continue
		}

		// 創建佣金支出
		salespersonName := "未知銷售員"
		if order.Salesperson != nil {
			salespersonName = order.Salesperson.Name
		}

		expense := models.Expense{
			TenantID:    tenantID,
			ExpenseType: "commission",
			Category:    "commission",
			Description: fmt.Sprintf("訂單佣金 - 訂單號: %s, 銷售員: %s", order.OrderNumber, salespersonName),
			Amount:      order.CommissionAmount,
			ExpenseDate: order.OrderDate,
			Status:      "pending",
			CreatedBy:   &userID,
			UpdatedBy:   &userID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			ExtraFields: models.JSONB(map[string]interface{}{
				"order_id":       order.ID.String(),
				"salesperson_id": order.SalespersonID.String(),
				"order_number":   order.OrderNumber,
			}),
		}

		if err := database.DB.Create(&expense).Error; err != nil {
			fmt.Printf("Failed to create commission expense for order %s: %v\n", order.OrderNumber, err)
			continue
		}

		createdCount++
	}

	return c.JSON(fiber.Map{
		"message": "Monthly commissions generated successfully",
		"count":   createdCount,
	})
}

func UpdateExpense(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid expense ID"})
	}

	var expense models.Expense
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&expense).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Expense not found"})
	}

	var req struct {
		ProjectID     *uuid.UUID             `json:"project_id"`
		RelatedUserID *uuid.UUID             `json:"related_user_id"`
		ExpenseType   string                 `json:"expense_type"`
		ReferenceID   *uuid.UUID             `json:"reference_id"`
		ReferenceType string                 `json:"reference_type"`
		Category      string                 `json:"category"`
		Description   string                 `json:"description"`
		Amount        float64                `json:"amount"`
		ExpenseDate   string                 `json:"expense_date"`
		PaymentMethod string                 `json:"payment_method"`
		BankAccountID *uuid.UUID             `json:"bank_account_id"`
		Vendor        string                 `json:"vendor"`
		Status        string                 `json:"status"`
		Notes         string                 `json:"notes"`
		ExtraFields   map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 設置 expense_type（如果沒有提供，使用 category）
	if req.ExpenseType == "" {
		req.ExpenseType = req.Category
		if req.ExpenseType == "" {
			req.ExpenseType = "other"
		}
	}

	// 根據類別設置 reference_type（例如 purchase -> purchase_order）
	if req.ReferenceType == "" {
		if req.Category == "purchase" {
			req.ReferenceType = "purchase_order"
		} else if req.Category == "project" {
			req.ReferenceType = "project"
		} else if req.Category == "refund" {
			req.ReferenceType = "order"
		}
	}

	// 更新字段
	expense.ProjectID = req.ProjectID
	expense.RelatedUserID = req.RelatedUserID
	expense.ExpenseType = req.ExpenseType
	expense.ReferenceID = req.ReferenceID
	expense.ReferenceType = req.ReferenceType
	expense.Category = req.Category
	expense.Description = req.Description
	expense.Amount = req.Amount
	expense.PaymentMethod = req.PaymentMethod
	expense.BankAccountID = req.BankAccountID
	expense.Vendor = req.Vendor
	expense.Status = req.Status
	expense.Notes = req.Notes
	expense.UpdatedBy = &userID
	expense.UpdatedAt = time.Now()

	if req.ExpenseDate != "" {
		if expenseDate, err := utils.ParseDateInTenantTimezone(tenantID, req.ExpenseDate); err == nil {
			expense.ExpenseDate = expenseDate
		}
	}

	if req.ExtraFields != nil {
		expense.ExtraFields = models.JSONB(req.ExtraFields)
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&expense).Error; err != nil {
			return err
		}
		if err := syncExpenseJournalEntryTx(tx, &expense, &userID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update expense"})
	}

	return c.JSON(expense)
}

func DeleteExpense(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid expense ID"})
	}

	var expense models.Expense
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&expense).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Expense not found"})
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := removeJournalEntryForSource(tx, tenantID, "expense", expense.ID); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.Expense{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete expense"})
	}

	return c.JSON(fiber.Map{"message": "Expense deleted successfully"})
}

// ============================================
// 會計統計
// ============================================

func GetAccountingSummary(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var totalIncome float64
	var totalExpense float64
	var purchaseTotal float64

	incomeQuery := database.DB.Model(&models.Income{}).Where("tenant_id = ? AND status = ?", tenantID, "confirmed")
	expenseQuery := database.DB.Model(&models.Expense{}).Where("tenant_id = ? AND status = ?", tenantID, "confirmed")
	purchaseQuery := database.DB.Model(&models.PurchaseOrder{}).Where("tenant_id = ? AND status = ?", tenantID, "completed")

	if startDate != "" {
		incomeQuery = incomeQuery.Where("income_date >= ?", startDate)
		expenseQuery = expenseQuery.Where("expense_date >= ?", startDate)
		purchaseQuery = purchaseQuery.Where("order_date >= ?", startDate)
	}
	if endDate != "" {
		incomeQuery = incomeQuery.Where("income_date <= ?", endDate)
		expenseQuery = expenseQuery.Where("expense_date <= ?", endDate)
		purchaseQuery = purchaseQuery.Where("order_date <= ?", endDate)
	}

	incomeQuery.Select("COALESCE(SUM(amount), 0)").Scan(&totalIncome)
	expenseQuery.Select("COALESCE(SUM(amount), 0)").Scan(&totalExpense)
	purchaseQuery.Select("COALESCE(SUM(final_amount), 0)").Scan(&purchaseTotal)

	netProfit := totalIncome - totalExpense - purchaseTotal

	// 計算每日數據（如果提供了日期範圍）
	type DailyStats struct {
		Date      string  `json:"date"`
		Income    float64 `json:"income"`
		Expense   float64 `json:"expense"`
		Purchase  float64 `json:"purchase"`
		NetProfit float64 `json:"net_profit"`
	}
	var dailyStats []DailyStats

	if startDate != "" && endDate != "" {
		start, err := utils.ParseDateInTenantTimezone(tenantID, startDate)
		if err == nil {
			end, err := utils.ParseDateInTenantTimezone(tenantID, endDate)
			if err == nil {
				// 確保在租戶時區中循環日期
				loc := utils.GetTenantLocation(tenantID)
				for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
					dateStr := d.In(loc).Format("2006-01-02")
					nextDay := d.AddDate(0, 0, 1).Format("2006-01-02")

					var dailyIncome float64
					var dailyExpense float64
					var dailyPurchase float64

					database.DB.Model(&models.Income{}).
						Where("tenant_id = ? AND status = ? AND income_date >= ? AND income_date < ?", tenantID, "confirmed", dateStr, nextDay).
						Select("COALESCE(SUM(amount), 0)").Scan(&dailyIncome)

					database.DB.Model(&models.Expense{}).
						Where("tenant_id = ? AND status = ? AND expense_date >= ? AND expense_date < ?", tenantID, "confirmed", dateStr, nextDay).
						Select("COALESCE(SUM(amount), 0)").Scan(&dailyExpense)

					database.DB.Model(&models.PurchaseOrder{}).
						Where("tenant_id = ? AND status = ? AND order_date >= ? AND order_date < ?", tenantID, "completed", dateStr, nextDay).
						Select("COALESCE(SUM(final_amount), 0)").Scan(&dailyPurchase)

					dailyStats = append(dailyStats, DailyStats{
						Date:      dateStr,
						Income:    dailyIncome,
						Expense:   dailyExpense,
						Purchase:  dailyPurchase,
						NetProfit: dailyIncome - dailyExpense - dailyPurchase,
					})
				}
			}
		}
	}

	return c.JSON(fiber.Map{
		"total_income":   totalIncome,
		"total_expense":  totalExpense,
		"purchase_total": purchaseTotal,
		"net_profit":     netProfit,
		"daily_chart":    dailyStats,
	})
}
