package handlers

import (
	"strconv"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type websitePageViewRow struct {
	PageID     uuid.UUID `json:"page_id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	IsHomepage bool      `json:"is_homepage"`
	Status     string    `json:"status"`
	Views      int       `json:"views"`       // 區間內（近 N 天）
	TotalViews int       `json:"total_views"` // 全期間
}

// GetWebsitePageViews 回傳每頁瀏覽量（近 N 天 + 總計）
// GET /api/v1/website/page-views?days=30&status=published
func GetWebsitePageViews(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	days := 30
	if s := strings.TrimSpace(c.Query("days")); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			days = v
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 3650 {
		days = 3650
	}

	// 區間：以 UTC 的「日期」為準
	now := time.Now().UTC()
	toDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	fromDate := toDate.AddDate(0, 0, -days+1)

	status := strings.TrimSpace(strings.ToLower(c.Query("status")))
	if status != "" && status != "draft" && status != "published" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid status. Must be 'draft' or 'published'."})
	}

	var rows []websitePageViewRow

	q := `
		SELECT
			p.id AS page_id,
			p.name,
			p.slug,
			p.is_homepage,
			p.status,
			COALESCE(SUM(CASE WHEN d.view_date >= ? AND d.view_date <= ? THEN d.view_count ELSE 0 END), 0) AS views,
			COALESCE(SUM(d.view_count), 0) AS total_views
		FROM pages p
		LEFT JOIN page_view_daily d
			ON d.tenant_id = p.tenant_id
			AND d.page_id = p.id
		WHERE p.tenant_id = ?
	`
	args := []interface{}{fromDate, toDate, tenantID}
	if status != "" {
		q += " AND p.status = ? "
		args = append(args, status)
	}
	q += `
		GROUP BY p.id
		ORDER BY views DESC, total_views DESC, p.created_at DESC
	`

	if err := database.DB.Raw(q, args...).Scan(&rows).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load page views"})
	}

	return c.JSON(fiber.Map{
		"days":      days,
		"from_date": fromDate.Format("2006-01-02"),
		"to_date":   toDate.Format("2006-01-02"),
		"data":      rows,
	})
}


