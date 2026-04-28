package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetDashboardStats 獲取儀表板統計數據
func GetDashboardStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	now := utils.NowInTenantTimezone(tenantID)
	loc := utils.GetTenantLocation(tenantID)
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	startOfYear := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)

	// 客戶統計
	var totalCustomers int64
	var activeCustomers int64
	database.DB.Model(&models.Customer{}).Where("tenant_id = ?", tenantID).Count(&totalCustomers)
	database.DB.Model(&models.Customer{}).Where("tenant_id = ? AND status = ?", tenantID, "active").Count(&activeCustomers)

	// 產品統計
	var totalProducts int64
	var activeProducts int64
	var lowStockProducts int64
	database.DB.Model(&models.Product{}).Where("tenant_id = ?", tenantID).Count(&totalProducts)
	database.DB.Model(&models.Product{}).Where("tenant_id = ? AND status = ?", tenantID, "active").Count(&activeProducts)
	database.DB.Model(&models.Product{}).Where("tenant_id = ? AND stock_quantity < 10", tenantID).Count(&lowStockProducts)

	// 訂單統計
	var totalOrders int64
	var pendingOrders int64
	var monthlyOrders int64
	var monthlyRevenue float64
	database.DB.Model(&models.Order{}).Where("tenant_id = ?", tenantID).Count(&totalOrders)
	database.DB.Model(&models.Order{}).Where("tenant_id = ? AND status IN ?", tenantID, []string{"draft", "confirmed", "processing"}).Count(&pendingOrders)
	database.DB.Model(&models.Order{}).Where("tenant_id = ? AND created_at >= ?", tenantID, startOfMonth).Count(&monthlyOrders)
	database.DB.Model(&models.Order{}).
		Where("tenant_id = ? AND created_at >= ? AND status != ?", tenantID, startOfMonth, "cancelled").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&monthlyRevenue)

	// 發票統計
	var totalInvoices int64
	var unpaidInvoices int64
	var monthlyInvoices int64
	var totalOutstanding float64
	database.DB.Model(&models.Invoice{}).Where("tenant_id = ?", tenantID).Count(&totalInvoices)
	database.DB.Model(&models.Invoice{}).Where("tenant_id = ? AND status IN ?", tenantID, []string{"draft", "sent", "overdue"}).Count(&unpaidInvoices)
	database.DB.Model(&models.Invoice{}).Where("tenant_id = ? AND created_at >= ?", tenantID, startOfMonth).Count(&monthlyInvoices)
	database.DB.Model(&models.Invoice{}).
		Where("tenant_id = ? AND status IN ?", tenantID, []string{"sent", "overdue"}).
		Select("COALESCE(SUM(total_amount - paid_amount), 0)").
		Scan(&totalOutstanding)

	// 年度收入
	var yearlyRevenue float64
	database.DB.Model(&models.Order{}).
		Where("tenant_id = ? AND created_at >= ? AND status != ?", tenantID, startOfYear, "cancelled").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&yearlyRevenue)

	// 最近訂單
	var recentOrders []models.Order
	database.DB.Where("tenant_id = ?", tenantID).
		Preload("Customer").
		Order("created_at DESC").
		Limit(5).
		Find(&recentOrders)

	// 最近預約
	var recentAppointments []models.Appointment
	database.DB.Where("tenant_id = ?", tenantID).
		Preload("Customer").
		Preload("Service").
		Preload("Staff").
		Order("start_time DESC").
		Limit(5).
		Find(&recentAppointments)

	// 獲取租戶信息
	var tenant models.Tenant
	database.DB.Where("id = ?", tenantID).First(&tenant)

	// 計算試用配額使用情況（用於免費版/試用版）
	var trialOrderCount int64
	var trialServiceOrderCount int64
	var trialTaskCount int64
	var aiTodayQueryCount int64
	database.DB.Model(&models.Order{}).Where("tenant_id = ?", tenantID).Count(&trialOrderCount)
	database.DB.Model(&models.ServiceOrder{}).Where("tenant_id = ?", tenantID).Count(&trialServiceOrderCount)
	database.DB.Model(&models.ProjectTask{}).Where("tenant_id = ?", tenantID).Count(&trialTaskCount)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	startOfNextDay := startOfDay.AddDate(0, 0, 1)
	database.DB.Model(&models.Message{}).
		Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND (message_type = ? OR extra_fields->>'ai_chat' = 'true')",
			tenantID, startOfDay, startOfNextDay, "ai_chat").
		Count(&aiTodayQueryCount)

	// 獲取本月每日的銷售、成本、profit 數據
	type DailyStats struct {
		Date   string  `json:"date"`
		Sales  float64 `json:"sales"`
		Cost   float64 `json:"cost"`
		Profit float64 `json:"profit"`
	}
	var dailyStats []DailyStats

	// 獲取本月所有天數
	daysInMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, loc).Day()

	for day := 1; day <= daysInMonth; day++ {
		date := time.Date(now.Year(), now.Month(), day, 0, 0, 0, 0, loc)
		nextDay := date.AddDate(0, 0, 1)

		var dailySales float64
		var dailyCost float64

		// 計算當日銷售額（訂單總額）
		database.DB.Model(&models.Order{}).
			Where("tenant_id = ? AND order_date >= ? AND order_date < ? AND status != ?",
				tenantID, date, nextDay, "cancelled").
			Select("COALESCE(SUM(total_amount), 0)").
			Scan(&dailySales)

		// 計算當日成本（訂單項目的成本總和）
		var orders []models.Order
		database.DB.Where("tenant_id = ? AND order_date >= ? AND order_date < ? AND status != ?",
			tenantID, date, nextDay, "cancelled").
			Preload("OrderItems").
			Preload("OrderItems.Product").
			Find(&orders)

		// 獲取該時間段內（本月及之前30天）的採購單產品平均價
		lookbackStart := startOfMonth.AddDate(0, 0, -30) // 往前看30天

		// 查詢採購單項目，計算每個產品的平均採購價
		var purchaseItems []models.PurchaseOrderItem
		database.DB.Joins("JOIN purchase_orders ON purchase_order_items.purchase_order_id = purchase_orders.id").
			Where("purchase_orders.tenant_id = ? AND purchase_orders.order_date >= ? AND purchase_orders.order_date <= ? AND purchase_order_items.product_id IS NOT NULL",
				tenantID, lookbackStart, nextDay).
			Find(&purchaseItems)

		// 計算每個產品的平均價（按產品ID分組）
		type ProductAvgPrice struct {
			ProductID uuid.UUID
			AvgPrice  float64
		}
		productPriceSum := make(map[uuid.UUID]float64)
		productPriceCount := make(map[uuid.UUID]int)

		for _, item := range purchaseItems {
			if item.ProductID != nil && item.UnitPrice > 0 {
				productPriceSum[*item.ProductID] += item.UnitPrice
				productPriceCount[*item.ProductID]++
			}
		}

		// 計算平均價
		avgPriceMap := make(map[uuid.UUID]float64)
		for productID, sum := range productPriceSum {
			if count := productPriceCount[productID]; count > 0 {
				avgPriceMap[productID] = sum / float64(count)
			}
		}

		for _, order := range orders {
			for _, item := range order.OrderItems {
				itemCost := 0.0
				if item.ProductID != nil {
					// 優先從採購單獲取平均價
					if avgPrice, exists := avgPriceMap[*item.ProductID]; exists && avgPrice > 0 {
						itemCost = avgPrice
					} else if item.Product != nil && item.Product.Cost > 0 {
						// 如果沒有採購單記錄，使用產品表中的成本
						itemCost = item.Product.Cost
					}
				}
				dailyCost += itemCost * float64(item.Quantity)
			}
		}

		dailyStats = append(dailyStats, DailyStats{
			Date:   date.Format("2006-01-02"),
			Sales:  dailySales,
			Cost:   dailyCost,
			Profit: dailySales - dailyCost,
		})
	}

	// 判斷是否為試用限制租戶
	isTrialLimited := (tenant.Plan == "trial" || tenant.Plan == "free") &&
		(tenant.SubscriptionID == nil || *tenant.SubscriptionID == "")

	return c.JSON(fiber.Map{
		"tenant": fiber.Map{
			"name":            tenant.Name,
			"plan":            tenant.Plan,
			"status":          tenant.Status,
			"subscription_id": tenant.SubscriptionID,
		},
		"trial_quota": fiber.Map{
			"is_trial_limited":     isTrialLimited,
			"orders_used":          trialOrderCount,
			"orders_limit":         5,
			"service_orders_used":  trialServiceOrderCount,
			"service_orders_limit": 5,
			"tasks_used":           trialTaskCount,
			"tasks_limit":          5,
			"ai_today_queries":     aiTodayQueryCount,
		},
		"customers": fiber.Map{
			"total":  totalCustomers,
			"active": activeCustomers,
		},
		"products": fiber.Map{
			"total":     totalProducts,
			"active":    activeProducts,
			"low_stock": lowStockProducts,
		},
		"orders": fiber.Map{
			"total":           totalOrders,
			"pending":         pendingOrders,
			"monthly_count":   monthlyOrders,
			"monthly_revenue": monthlyRevenue,
			"yearly_revenue":  yearlyRevenue,
		},
		"invoices": fiber.Map{
			"total":         totalInvoices,
			"unpaid":        unpaidInvoices,
			"monthly_count": monthlyInvoices,
			"outstanding":   totalOutstanding,
		},
		"recent_orders":       recentOrders,
		"recent_appointments": recentAppointments,
		"monthly_chart":       dailyStats,
	})
}
