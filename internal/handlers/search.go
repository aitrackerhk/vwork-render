package handlers

import (
	"strings"

	"nwork/internal/database"
	"nwork/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type GlobalSearchItem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Code     string `json:"code,omitempty"`
	URL      string `json:"url"`
}

// GlobalSearch CMS 全站搜尋：客戶/項目/使用者/訂單/服務單/採購單
func GlobalSearch(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return c.JSON(fiber.Map{"data": []GlobalSearchItem{}})
	}

	limit := c.QueryInt("limit", 20)
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	like := "%" + q + "%"
	perType := 6
	if limit < perType {
		perType = limit
	}

	items := make([]GlobalSearchItem, 0, limit)

	// Customers
	{
		type row struct {
			ID   uuid.UUID
			Code string
			Name string
			Phone string
		}
		var rows []row
		database.DB.Table("customers").
			Select("id, code, name, phone").
			Where("tenant_id = ? AND (code ILIKE ? OR name ILIKE ? OR phone ILIKE ?)", tenantID, like, like, like).
			Order("created_at DESC").
			Limit(perType).
			Find(&rows)
		for _, r := range rows {
			title := r.Name
			if strings.TrimSpace(title) == "" {
				title = r.Code
			}
			sub := strings.TrimSpace(r.Phone)
			if sub == "" && r.Code != "" && r.Code != title {
				sub = r.Code
			}
			items = append(items, GlobalSearchItem{
				Type:     "customer",
				Title:    title,
				Subtitle: sub,
				Code:     r.Code,
				URL:      "/customers/" + r.ID.String() + "/edit",
			})
		}
	}

	// Projects
	{
		type row struct {
			ID   uuid.UUID
			Code string
			Name string
		}
		var rows []row
		database.DB.Table("projects").
			Select("id, code, name").
			Where("tenant_id = ? AND (code ILIKE ? OR name ILIKE ?)", tenantID, like, like).
			Order("created_at DESC").
			Limit(perType).
			Find(&rows)
		for _, r := range rows {
			title := r.Name
			if strings.TrimSpace(title) == "" {
				title = r.Code
			}
			sub := ""
			if r.Code != "" && r.Code != title {
				sub = r.Code
			}
			items = append(items, GlobalSearchItem{
				Type:     "project",
				Title:    title,
				Subtitle: sub,
				Code:     r.Code,
				URL:      "/projects/" + r.ID.String() + "/edit",
			})
		}
	}

	// Users
	{
		type row struct {
			ID            uuid.UUID
			Name          string
			Email         string
			EmployeeNumber string
		}
		var rows []row
		database.DB.Table("users").
			Select("id, name, email, employee_number").
			Where("tenant_id = ? AND (employee_number ILIKE ? OR name ILIKE ? OR email ILIKE ?)", tenantID, like, like, like).
			Order("created_at DESC").
			Limit(perType).
			Find(&rows)
		for _, r := range rows {
			title := strings.TrimSpace(r.Name)
			if title == "" {
				title = strings.TrimSpace(r.Email)
			}
			if title == "" {
				title = r.ID.String()
			}
			sub := strings.TrimSpace(r.EmployeeNumber)
			if sub == "" {
				sub = strings.TrimSpace(r.Email)
			}
			items = append(items, GlobalSearchItem{
				Type:     "user",
				Title:    title,
				Subtitle: sub,
				Code:     r.EmployeeNumber,
				URL:      "/users/" + r.ID.String() + "/edit",
			})
		}
	}

	// Orders
	{
		type row struct {
			ID         uuid.UUID
			OrderNumber string
		}
		var rows []row
		database.DB.Table("orders").
			Select("id, order_number").
			Where("tenant_id = ? AND order_number ILIKE ?", tenantID, like).
			Order("created_at DESC").
			Limit(perType).
			Find(&rows)
		for _, r := range rows {
			title := r.OrderNumber
			if strings.TrimSpace(title) == "" {
				title = r.ID.String()
			}
			items = append(items, GlobalSearchItem{
				Type:  "order",
				Title: title,
				Code:  r.OrderNumber,
				URL:   "/orders/" + r.ID.String() + "/edit",
			})
		}
	}

	// Service Orders
	{
		type row struct {
			ID         uuid.UUID
			OrderNumber string
		}
		var rows []row
		database.DB.Table("service_orders").
			Select("id, order_number").
			Where("tenant_id = ? AND order_number ILIKE ?", tenantID, like).
			Order("created_at DESC").
			Limit(perType).
			Find(&rows)
		for _, r := range rows {
			title := r.OrderNumber
			if strings.TrimSpace(title) == "" {
				title = r.ID.String()
			}
			items = append(items, GlobalSearchItem{
				Type:  "service_order",
				Title: title,
				Code:  r.OrderNumber,
				URL:   "/service-orders/" + r.ID.String() + "/edit",
			})
		}
	}

	// Purchase Orders
	{
		type row struct {
			ID         uuid.UUID
			OrderNumber string
		}
		var rows []row
		database.DB.Table("purchase_orders").
			Select("id, order_number").
			Where("tenant_id = ? AND order_number ILIKE ?", tenantID, like).
			Order("created_at DESC").
			Limit(perType).
			Find(&rows)
		for _, r := range rows {
			title := r.OrderNumber
			if strings.TrimSpace(title) == "" {
				title = r.ID.String()
			}
			items = append(items, GlobalSearchItem{
				Type:  "purchase_order",
				Title: title,
				Code:  r.OrderNumber,
				URL:   "/purchase-orders/" + r.ID.String() + "/edit",
			})
		}
	}

	// overall cap
	if len(items) > limit {
		items = items[:limit]
	}

	return c.JSON(fiber.Map{"data": items})
}


