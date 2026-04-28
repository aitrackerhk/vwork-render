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

// GetLatestCustomers 獲取最新的N個客人（用於vAI查詢）
func GetLatestCustomers(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// 從查詢參數獲取數量，默認為5
	limit := c.QueryInt("limit", 5)
	if limit > 100 {
		limit = 100 // 最大限制100
	}
	if limit < 1 {
		limit = 5
	}

	var customers []models.Customer
	query := database.DB.Where("tenant_id = ?", tenantID).
		Preload("MemberLevel").
		Order("created_at DESC").
		Limit(limit)

	if err := query.Find(&customers).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch customers: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  customers,
		"count": len(customers),
		"limit": limit,
	})
}

// GetTopCustomersBySpending 獲取購物最多的N個客人（按訂單總金額排序，用於vAI查詢）
func GetTopCustomersBySpending(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// 從查詢參數獲取數量，默認為5
	limit := c.QueryInt("limit", 5)
	if limit > 100 {
		limit = 100 // 最大限制100
	}
	if limit < 1 {
		limit = 5
	}

	type CustomerSpending struct {
		Customer      models.Customer `json:"customer"`
		TotalSpending float64         `json:"total_spending"`
		OrderCount    int64           `json:"order_count"`
	}

	var results []CustomerSpending

	// 使用子查詢統計每個客戶的總消費金額
	subQuery := database.DB.Model(&models.Order{}).
		Select("customer_id, SUM(total_amount) as total_spending, COUNT(*) as order_count").
		Where("tenant_id = ? AND customer_id IS NOT NULL AND status != ?", tenantID, "quotation").
		Group("customer_id").
		Order("total_spending DESC").
		Limit(limit)

	rows, err := subQuery.Rows()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch customer spending: %v", err)})
	}
	defer rows.Close()

	customerIDs := []uuid.UUID{}
	customerSpendingMap := make(map[uuid.UUID]float64)
	customerOrderCountMap := make(map[uuid.UUID]int64)

	for rows.Next() {
		var customerID uuid.UUID
		var totalSpending float64
		var orderCount int64

		if err := rows.Scan(&customerID, &totalSpending, &orderCount); err != nil {
			continue
		}

		customerIDs = append(customerIDs, customerID)
		customerSpendingMap[customerID] = totalSpending
		customerOrderCountMap[customerID] = orderCount
	}

	// 如果沒有找到任何客戶，返回空結果
	if len(customerIDs) == 0 {
		return c.JSON(fiber.Map{
			"data":  []CustomerSpending{},
			"count": 0,
			"limit": limit,
		})
	}

	// 批量查詢客戶信息
	var customers []models.Customer
	if err := database.DB.Where("tenant_id = ? AND id IN ?", tenantID, customerIDs).
		Preload("MemberLevel").
		Find(&customers).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch customers: %v", err)})
	}

	// 創建客戶ID到客戶對象的映射
	customerMap := make(map[uuid.UUID]models.Customer)
	for _, customer := range customers {
		customerMap[customer.ID] = customer
	}

	// 按照總消費金額排序的順序組裝結果
	for _, customerID := range customerIDs {
		if customer, exists := customerMap[customerID]; exists {
			results = append(results, CustomerSpending{
				Customer:      customer,
				TotalSpending: customerSpendingMap[customerID],
				OrderCount:    customerOrderCountMap[customerID],
			})
		}
	}

	return c.JSON(fiber.Map{
		"data":  results,
		"count": len(results),
		"limit": limit,
	})
}

// GetLargestOrder 獲取金額最大的訂單（用於vAI查詢）
func GetLargestOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// 從查詢參數獲取數量，默認為1（最大的一張訂單）
	limit := c.QueryInt("limit", 1)
	if limit > 10 {
		limit = 10 // 最大限制10
	}
	if limit < 1 {
		limit = 1
	}

	var orders []models.Order
	query := database.DB.Where("tenant_id = ? AND status != ?", tenantID, "quotation").
		Preload("Customer").
		Preload("OrderItems.Product").
		Preload("Labels").
		Order("total_amount DESC").
		Limit(limit)

	if err := query.Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch orders: %v", err)})
	}

	if len(orders) == 0 {
		return c.JSON(fiber.Map{
			"data":  []models.Order{},
			"count": 0,
			"limit": limit,
		})
	}

	return c.JSON(fiber.Map{
		"data":  orders,
		"count": len(orders),
		"limit": limit,
	})
}

// GetVAIStats 獲取vAI查詢常用的統計數據（綜合查詢接口）
func GetVAIStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// 獲取最新的5個客人
	var latestCustomers []models.Customer
	database.DB.Where("tenant_id = ?", tenantID).
		Preload("MemberLevel").
		Order("created_at DESC").
		Limit(5).
		Find(&latestCustomers)

	// 獲取購物最多的5個客人
	type CustomerSpending struct {
		CustomerID    uuid.UUID       `json:"customer_id"`
		Customer      models.Customer `json:"customer"`
		TotalSpending float64         `json:"total_spending"`
		OrderCount    int64           `json:"order_count"`
	}

	var topCustomers []CustomerSpending
	rows, err := database.DB.Model(&models.Order{}).
		Select("customer_id, SUM(total_amount) as total_spending, COUNT(*) as order_count").
		Where("tenant_id = ? AND customer_id IS NOT NULL AND status != ?", tenantID, "quotation").
		Group("customer_id").
		Order("total_spending DESC").
		Limit(5).
		Rows()

	if err == nil {
		defer rows.Close()
		customerIDs := []uuid.UUID{}
		spendingMap := make(map[uuid.UUID]float64)
		orderCountMap := make(map[uuid.UUID]int64)

		for rows.Next() {
			var customerID uuid.UUID
			var totalSpending float64
			var orderCount int64
			if err := rows.Scan(&customerID, &totalSpending, &orderCount); err == nil {
				customerIDs = append(customerIDs, customerID)
				spendingMap[customerID] = totalSpending
				orderCountMap[customerID] = orderCount
			}
		}

		if len(customerIDs) > 0 {
			var customers []models.Customer
			database.DB.Where("tenant_id = ? AND id IN ?", tenantID, customerIDs).
				Preload("MemberLevel").
				Find(&customers)

			customerMap := make(map[uuid.UUID]models.Customer)
			for _, customer := range customers {
				customerMap[customer.ID] = customer
			}

			for _, customerID := range customerIDs {
				if customer, exists := customerMap[customerID]; exists {
					topCustomers = append(topCustomers, CustomerSpending{
						CustomerID:    customerID,
						Customer:      customer,
						TotalSpending: spendingMap[customerID],
						OrderCount:    orderCountMap[customerID],
					})
				}
			}
		}
	}

	// 獲取金額最大的訂單
	var largestOrder models.Order
	database.DB.Where("tenant_id = ? AND status != ?", tenantID, "quotation").
		Preload("Customer").
		Preload("OrderItems.Product").
		Preload("Labels").
		Order("total_amount DESC").
		First(&largestOrder)

	// 總體統計
	var totalCustomers int64
	var totalOrders int64
	var totalRevenue float64

	database.DB.Model(&models.Customer{}).Where("tenant_id = ?", tenantID).Count(&totalCustomers)
	database.DB.Model(&models.Order{}).
		Where("tenant_id = ? AND status != ?", tenantID, "quotation").
		Count(&totalOrders)

	database.DB.Model(&models.Order{}).
		Where("tenant_id = ? AND status != ?", tenantID, "quotation").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&totalRevenue)

	return c.JSON(fiber.Map{
		"latest_customers": latestCustomers,
		"top_customers":    topCustomers,
		"largest_order":    largestOrder,
		"total_customers":  totalCustomers,
		"total_orders":     totalOrders,
		"total_revenue":    totalRevenue,
	})
}

// GetVAIHolidays 獲取假期（用於 vAI 查詢）
func GetVAIHolidays(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	startDate, endDate, err := parseDateRange(c, 30)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	limit := c.QueryInt("limit", 10)
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 10
	}

	var holidays []models.Holiday
	if err := database.DB.Where("tenant_id = ? AND status = ? AND start_date <= ? AND end_date >= ?",
		tenantID, "active", endDate, startDate).
		Order("start_date ASC").
		Limit(limit).
		Find(&holidays).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch holidays: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  holidays,
		"count": len(holidays),
		"limit": limit,
		"start": startDate.Format("2006-01-02"),
		"end":   endDate.Format("2006-01-02"),
	})
}

// GetVAIAppointments 獲取預約（用於 vAI 查詢）
func GetVAIAppointments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	startAt, endAt, err := parseDateTimeRange(c, 7)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	limit := c.QueryInt("limit", 20)
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 20
	}

	query := database.DB.Where("tenant_id = ? AND start_time >= ? AND start_time <= ?",
		tenantID, startAt, endAt)

	if staffID := c.Query("staff_id"); staffID != "" {
		id, err := uuid.Parse(staffID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid staff_id"})
		}
		query = query.Where("staff_id = ?", id)
	}
	if customerID := c.Query("customer_id"); customerID != "" {
		id, err := uuid.Parse(customerID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid customer_id"})
		}
		query = query.Where("customer_id = ?", id)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var appointments []models.Appointment
	if err := query.Preload("Customer").
		Preload("Service").
		Preload("Staff").
		Preload("Rooms").
		Preload("Equipments").
		Preload("Vehicles").
		Order("start_time ASC").
		Limit(limit).
		Find(&appointments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch appointments: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  appointments,
		"count": len(appointments),
		"limit": limit,
		"start": startAt.Format(time.RFC3339),
		"end":   endAt.Format(time.RFC3339),
	})
}

// GetVAIStaffShifts 獲取員工排工/班表（用於 vAI 查詢）
func GetVAIStaffShifts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	limit := c.QueryInt("limit", 20)
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 20
	}

	query := database.DB.Where("tenant_id = ?", tenantID)
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if shiftID := c.Query("shift_id"); shiftID != "" {
		id, err := uuid.Parse(shiftID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid shift_id"})
		}
		query = query.Where("shift_id = ?", id)
	}

	var users []models.User
	if err := query.Preload("Department").
		Preload("Shift").
		Order("created_at DESC").
		Limit(limit).
		Find(&users).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch staff shifts: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  users,
		"count": len(users),
		"limit": limit,
	})
}

func parseDateRange(c *fiber.Ctx, defaultDays int) (time.Time, time.Time, error) {
	startStr := c.Query("start")
	endStr := c.Query("end")
	loc := time.Now().Location()

	var start time.Time
	var end time.Time
	if startStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", startStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("Invalid start date")
		}
		start = parsed
	} else {
		now := time.Now().In(loc)
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}

	if endStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", endStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("Invalid end date")
		}
		end = parsed
	} else {
		end = start.AddDate(0, 0, defaultDays)
	}

	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("End date must be after start date")
	}
	return start, end, nil
}

func parseDateTimeRange(c *fiber.Ctx, defaultDays int) (time.Time, time.Time, error) {
	startStr := c.Query("start")
	endStr := c.Query("end")
	loc := time.Now().Location()

	var start time.Time
	var end time.Time
	if startStr != "" {
		parsed, err := parseDateOrDateTime(startStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("Invalid start datetime")
		}
		start = parsed
	} else {
		now := time.Now().In(loc)
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}

	if endStr != "" {
		parsed, err := parseDateOrDateTime(endStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("Invalid end datetime")
		}
		end = parsed
	} else {
		end = start.AddDate(0, 0, defaultDays)
	}

	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("End datetime must be after start datetime")
	}
	return start, end, nil
}

func parseDateOrDateTime(value string, loc *time.Location) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.In(loc), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", value, loc); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid datetime")
}

// ============ 會計相關 vAI API ============

// GetVAILatestTransactions 獲取最新的交易記錄（收入+支出）
func GetVAILatestTransactions(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	limit := c.QueryInt("limit", 10)
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 10
	}

	// 獲取最新收入
	var incomes []models.Income
	database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).
		Order("income_date DESC, created_at DESC").
		Limit(limit).
		Find(&incomes)

	// 獲取最新支出
	var expenses []models.Expense
	database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).
		Order("expense_date DESC, created_at DESC").
		Limit(limit).
		Find(&expenses)

	return c.JSON(fiber.Map{
		"incomes":  incomes,
		"expenses": expenses,
		"limit":    limit,
	})
}

// GetVAIIncomeSummary 獲取收入總結
func GetVAIIncomeSummary(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// 默認查詢本月
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

	var totalIncome float64
	database.DB.Model(&models.Income{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND income_date >= ? AND income_date <= ?",
			tenantID, startOfMonth, endOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalIncome)

	// 按類別統計
	type CategorySum struct {
		Category string  `json:"category"`
		Total    float64 `json:"total"`
	}
	var byCategory []CategorySum
	database.DB.Model(&models.Income{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND income_date >= ? AND income_date <= ?",
			tenantID, startOfMonth, endOfMonth).
		Select("COALESCE(category, income_type) as category, SUM(amount) as total").
		Group("COALESCE(category, income_type)").
		Scan(&byCategory)

	return c.JSON(fiber.Map{
		"total_income": totalIncome,
		"by_category":  byCategory,
		"period":       fmt.Sprintf("%s 至 %s", startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02")),
	})
}

// GetVAIExpenseSummary 獲取支出總結
func GetVAIExpenseSummary(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// 默認查詢本月
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

	var totalExpense float64
	database.DB.Model(&models.Expense{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND expense_date >= ? AND expense_date <= ?",
			tenantID, startOfMonth, endOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalExpense)

	// 按類別統計
	type CategorySum struct {
		Category string  `json:"category"`
		Total    float64 `json:"total"`
	}
	var byCategory []CategorySum
	database.DB.Model(&models.Expense{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND expense_date >= ? AND expense_date <= ?",
			tenantID, startOfMonth, endOfMonth).
		Select("category, SUM(amount) as total").
		Group("category").
		Scan(&byCategory)

	return c.JSON(fiber.Map{
		"total_expense": totalExpense,
		"by_category":   byCategory,
		"period":        fmt.Sprintf("%s 至 %s", startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02")),
	})
}

// GetVAIAccountsReceivable 獲取應收帳款（未付款的發票）
func GetVAIAccountsReceivable(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	limit := c.QueryInt("limit", 10)
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 10
	}

	var invoices []models.Invoice
	database.DB.Where("tenant_id = ? AND status IN (?, ?)", tenantID, "pending", "partial").
		Preload("Customer").
		Order("due_date ASC").
		Limit(limit).
		Find(&invoices)

	var totalReceivable float64
	database.DB.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status IN (?, ?)", tenantID, "pending", "partial").
		Select("COALESCE(SUM(total_amount - paid_amount), 0)").
		Scan(&totalReceivable)

	return c.JSON(fiber.Map{
		"data":             invoices,
		"total_receivable": totalReceivable,
		"count":            len(invoices),
		"limit":            limit,
	})
}

// GetVAIAccountsPayable 獲取應付帳款（待付款的支出）
func GetVAIAccountsPayable(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	limit := c.QueryInt("limit", 10)
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 10
	}

	var expenses []models.Expense
	database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND status = ?", tenantID, "pending").
		Order("expense_date ASC").
		Limit(limit).
		Find(&expenses)

	var totalPayable float64
	database.DB.Model(&models.Expense{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = ?", tenantID, "pending").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalPayable)

	return c.JSON(fiber.Map{
		"data":          expenses,
		"total_payable": totalPayable,
		"count":         len(expenses),
		"limit":         limit,
	})
}

// GetVAIProfitLoss 獲取損益報表（本月收入-支出）
func GetVAIProfitLoss(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

	var totalIncome float64
	database.DB.Model(&models.Income{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND income_date >= ? AND income_date <= ?",
			tenantID, startOfMonth, endOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalIncome)

	var totalExpense float64
	database.DB.Model(&models.Expense{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND expense_date >= ? AND expense_date <= ?",
			tenantID, startOfMonth, endOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalExpense)

	netProfit := totalIncome - totalExpense

	return c.JSON(fiber.Map{
		"total_income":  totalIncome,
		"total_expense": totalExpense,
		"net_profit":    netProfit,
		"period":        fmt.Sprintf("%s 至 %s", startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02")),
	})
}

// GetVAIBalanceSheet 獲取資產負債表（簡化版）
func GetVAIBalanceSheet(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// 應收帳款
	var accountsReceivable float64
	database.DB.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status IN (?, ?)", tenantID, "pending", "partial").
		Select("COALESCE(SUM(total_amount - paid_amount), 0)").
		Scan(&accountsReceivable)

	// 應付帳款
	var accountsPayable float64
	database.DB.Model(&models.Expense{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = ?", tenantID, "pending").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&accountsPayable)

	// 累計收入（所有已確認收入）
	var totalIncome float64
	database.DB.Model(&models.Income{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed'", tenantID).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalIncome)

	// 累計支出（所有已確認支出）
	var totalExpense float64
	database.DB.Model(&models.Expense{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed'", tenantID).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalExpense)

	retainedEarnings := totalIncome - totalExpense

	return c.JSON(fiber.Map{
		"assets": fiber.Map{
			"accounts_receivable": accountsReceivable,
		},
		"liabilities": fiber.Map{
			"accounts_payable": accountsPayable,
		},
		"equity": fiber.Map{
			"retained_earnings": retainedEarnings,
		},
	})
}

// GetVAIBusinessGoals 獲取業務目標（用於 vAI 查詢）
func GetVAIBusinessGoals(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	limit := c.QueryInt("limit", 10)
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 10
	}

	status := c.Query("status")
	query := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)

	if status != "" {
		query = query.Where("status = ?", status)
	} else {
		// Default: show active goals
		query = query.Where("status = ?", "active")
	}

	var goals []models.BusinessGoal
	if err := query.Order("priority DESC, end_date ASC").
		Limit(limit).
		Find(&goals).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch business goals: %v", err)})
	}

	// Build summary with progress
	type GoalSummary struct {
		ID              uuid.UUID `json:"id"`
		Title           string    `json:"title"`
		Description     string    `json:"description"`
		MetricType      string    `json:"metric_type"`
		TargetValue     float64   `json:"target_value"`
		CurrentValue    float64   `json:"current_value"`
		Unit            string    `json:"unit"`
		ProgressPercent float64   `json:"progress_percent"`
		StartDate       string    `json:"start_date"`
		EndDate         string    `json:"end_date"`
		Status          string    `json:"status"`
		Priority        string    `json:"priority"`
		DaysRemaining   int       `json:"days_remaining"`
	}

	summaries := make([]GoalSummary, 0, len(goals))
	now := time.Now()
	for _, g := range goals {
		daysRemaining := int(g.EndDate.Sub(now).Hours() / 24)
		if daysRemaining < 0 {
			daysRemaining = 0
		}
		summaries = append(summaries, GoalSummary{
			ID:              g.ID,
			Title:           g.Title,
			Description:     g.Description,
			MetricType:      g.MetricType,
			TargetValue:     g.TargetValue,
			CurrentValue:    g.CurrentValue,
			Unit:            g.Unit,
			ProgressPercent: g.ProgressPercent(),
			StartDate:       g.StartDate.Format("2006-01-02"),
			EndDate:         g.EndDate.Format("2006-01-02"),
			Status:          g.Status,
			Priority:        g.Priority,
			DaysRemaining:   daysRemaining,
		})
	}

	// Overall stats
	var totalActive int64
	var totalCompleted int64
	database.DB.Model(&models.BusinessGoal{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = ?", tenantID, "active").
		Count(&totalActive)
	database.DB.Model(&models.BusinessGoal{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = ?", tenantID, "completed").
		Count(&totalCompleted)

	return c.JSON(fiber.Map{
		"data":            summaries,
		"count":           len(summaries),
		"limit":           limit,
		"total_active":    totalActive,
		"total_completed": totalCompleted,
	})
}

// GetVAICashFlow 獲取現金流量（本月）
func GetVAICashFlow(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

	// 現金收入
	var cashInflow float64
	database.DB.Model(&models.Income{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND income_date >= ? AND income_date <= ?",
			tenantID, startOfMonth, endOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&cashInflow)

	// 現金支出
	var cashOutflow float64
	database.DB.Model(&models.Expense{}).
		Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND expense_date >= ? AND expense_date <= ?",
			tenantID, startOfMonth, endOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&cashOutflow)

	netCashFlow := cashInflow - cashOutflow

	return c.JSON(fiber.Map{
		"cash_inflow":   cashInflow,
		"cash_outflow":  cashOutflow,
		"net_cash_flow": netCashFlow,
		"period":        fmt.Sprintf("%s 至 %s", startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02")),
	})
}
