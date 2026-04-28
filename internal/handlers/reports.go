package handlers

import (
	"time"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/utils"

	"github.com/gofiber/fiber/v2"
)

type customerTopRow struct {
	CustomerID   string  `json:"customer_id"`
	CustomerName string  `json:"customer_name"`
	Phone        string  `json:"phone"`
	OrdersCount  int64   `json:"orders_count"`
	Revenue      float64 `json:"revenue"`
	IsTrashed    bool    `json:"is_trashed"`
}

// GetCustomerAnalysisReport 客戶分析報告（簡版）
func GetCustomerAnalysisReport(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	loc := utils.GetTenantLocation(tenantID)

	// default: last 30 days
	end := utils.NowInTenantTimezone(tenantID)
	start := end.AddDate(0, 0, -30)

	if s := c.Query("start_date"); s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			start = t
		}
	}
	if s := c.Query("end_date"); s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			// include whole day
			end = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		}
	}

	var totalCustomers int64
	_ = database.DB.Raw(`SELECT COUNT(1) FROM customers WHERE tenant_id = ? AND trashed_at IS NULL`, tenantID).Scan(&totalCustomers).Error

	var newCustomers int64
	_ = database.DB.Raw(`
		SELECT COUNT(1)
		FROM customers
		WHERE tenant_id = ?
		  AND trashed_at IS NULL
		  AND created_at >= ?
		  AND created_at <= ?
	`, tenantID, start, end).Scan(&newCustomers).Error

	var ordersRevenue float64
	var ordersCount int64
	_ = database.DB.Raw(`
		SELECT COALESCE(SUM(total_amount), 0) AS revenue, COUNT(1) AS cnt
		FROM orders
		WHERE tenant_id = ?
		  AND trashed_at IS NULL
		  AND order_date >= ?
		  AND order_date <= ?
		  AND status != 'cancelled'
	`, tenantID, start, end).Row().Scan(&ordersRevenue, &ordersCount)

	var servicesRevenue float64
	var servicesCount int64
	_ = database.DB.Raw(`
		SELECT COALESCE(SUM(total_amount), 0) AS revenue, COUNT(1) AS cnt
		FROM service_orders
		WHERE tenant_id = ?
		  AND trashed_at IS NULL
		  AND service_date >= ?
		  AND service_date <= ?
		  AND status != 'cancelled'
	`, tenantID, start, end).Row().Scan(&servicesRevenue, &servicesCount)

	// Top customers: exclude trashed orders/service_orders entirely;
	// include trashed customers ONLY if they have non-trashed orders, mark them as is_trashed;
	// sort trashed customers to the bottom.
	top := []customerTopRow{}
	_ = database.DB.Raw(`
		WITH ord AS (
			SELECT customer_id, COUNT(1) AS cnt, COALESCE(SUM(total_amount), 0) AS revenue
			FROM orders
			WHERE tenant_id = ?
			  AND trashed_at IS NULL
			  AND customer_id IS NOT NULL
			  AND order_date >= ?
			  AND order_date <= ?
			  AND status != 'cancelled'
			GROUP BY customer_id
		),
		srv AS (
			SELECT customer_id, COUNT(1) AS cnt, COALESCE(SUM(total_amount), 0) AS revenue
			FROM service_orders
			WHERE tenant_id = ?
			  AND trashed_at IS NULL
			  AND customer_id IS NOT NULL
			  AND service_date >= ?
			  AND service_date <= ?
			  AND status != 'cancelled'
			GROUP BY customer_id
		)
		SELECT
			c.id::text AS customer_id,
			COALESCE(c.name, '') AS customer_name,
			COALESCE(c.phone, '') AS phone,
			COALESCE(ord.cnt, 0) + COALESCE(srv.cnt, 0) AS orders_count,
			COALESCE(ord.revenue, 0) + COALESCE(srv.revenue, 0) AS revenue,
			(c.trashed_at IS NOT NULL) AS is_trashed
		FROM customers c
		LEFT JOIN ord ON ord.customer_id = c.id
		LEFT JOIN srv ON srv.customer_id = c.id
		WHERE c.tenant_id = ?
		  AND (c.trashed_at IS NULL OR COALESCE(ord.cnt, 0) + COALESCE(srv.cnt, 0) > 0)
		ORDER BY (c.trashed_at IS NOT NULL), revenue DESC
		LIMIT 10
	`, tenantID, start, end, tenantID, start, end, tenantID).Scan(&top).Error

	return c.JSON(fiber.Map{
		"range": fiber.Map{
			"start_date": start.Format("2006-01-02"),
			"end_date":   end.Format("2006-01-02"),
		},
		"summary": fiber.Map{
			"total_customers":        totalCustomers,
			"new_customers":          newCustomers,
			"orders_count":           ordersCount,
			"orders_revenue":         ordersRevenue,
			"service_orders_count":   servicesCount,
			"service_orders_revenue": servicesRevenue,
		},
		"top_customers": top,
	})
}

type projectTopExpenseRow struct {
	ProjectID   string  `json:"project_id"`
	ProjectName string  `json:"project_name"`
	Expense     float64 `json:"expense"`
}

// GetProjectAnalysisReport 項目分析報告（簡版）
func GetProjectAnalysisReport(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	loc := utils.GetTenantLocation(tenantID)

	end := utils.NowInTenantTimezone(tenantID)
	start := end.AddDate(0, 0, -30)
	if s := c.Query("start_date"); s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			start = t
		}
	}
	if s := c.Query("end_date"); s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			end = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		}
	}

	var totalProjects int64
	var activeProjects int64
	var completedProjects int64
	_ = database.DB.Raw(`SELECT COUNT(1) FROM projects WHERE tenant_id = ?`, tenantID).Scan(&totalProjects).Error
	_ = database.DB.Raw(`SELECT COUNT(1) FROM projects WHERE tenant_id = ? AND status = 'active'`, tenantID).Scan(&activeProjects).Error
	_ = database.DB.Raw(`SELECT COUNT(1) FROM projects WHERE tenant_id = ? AND status = 'completed'`, tenantID).Scan(&completedProjects).Error

	// overdue tasks
	today := utils.NowInTenantTimezone(tenantID).In(loc).Truncate(24 * time.Hour)
	var overdueTasks int64
	_ = database.DB.Raw(`
		SELECT COUNT(1)
		FROM project_tasks
		WHERE tenant_id = ?
		  AND due_date IS NOT NULL
		  AND due_date < ?
		  AND status != 'done'
	`, tenantID, today).Scan(&overdueTasks).Error

	// projects ending today
	var endingToday int64
	_ = database.DB.Raw(`
		SELECT COUNT(1)
		FROM projects
		WHERE tenant_id = ?
		  AND end_date = ?
	`, tenantID, today.Format("2006-01-02")).Scan(&endingToday).Error

	// expenses in range
	var totalExpense float64
	_ = database.DB.Raw(`
		SELECT COALESCE(SUM(amount), 0)
		FROM expenses
		WHERE tenant_id = ?
		  AND project_id IS NOT NULL
		  AND expense_date >= ?
		  AND expense_date <= ?
		  AND status != 'cancelled'
	`, tenantID, start, end).Scan(&totalExpense).Error

	top := []projectTopExpenseRow{}
	_ = database.DB.Raw(`
		SELECT p.id::text AS project_id, p.name AS project_name, COALESCE(SUM(e.amount), 0) AS expense
		FROM projects p
		JOIN expenses e ON e.project_id = p.id AND e.tenant_id = p.tenant_id
		WHERE p.tenant_id = ?
		  AND e.expense_date >= ?
		  AND e.expense_date <= ?
		  AND e.status != 'cancelled'
		GROUP BY p.id, p.name
		ORDER BY expense DESC
		LIMIT 10
	`, tenantID, start, end).Scan(&top).Error

	return c.JSON(fiber.Map{
		"range": fiber.Map{
			"start_date": start.Format("2006-01-02"),
			"end_date":   end.Format("2006-01-02"),
		},
		"summary": fiber.Map{
			"total_projects":     totalProjects,
			"active_projects":    activeProjects,
			"completed_projects": completedProjects,
			"overdue_tasks":      overdueTasks,
			"ending_today":       endingToday,
			"project_expense":    totalExpense,
		},
		"top_project_expenses": top,
	})
}
