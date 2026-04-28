package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

func parseBoolQuery(c *fiber.Ctx, key string) bool {
	v := strings.ToLower(strings.TrimSpace(c.Query(key)))
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

func parseIntQuery(c *fiber.Ctx, key string, def int, max int) int {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < 1 {
		return def
	}
	if max > 0 && n > max {
		return max
	}
	return n
}

func writeTemplateExcel(c *fiber.Ctx, filename string, headers []string, rows [][]interface{}, instructions []string) error {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Import"
	f.SetSheetName("Sheet1", sheet)

	// header row
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	// bold header
	if len(headers) > 0 {
		style, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
		f.SetCellStyle(sheet, "A1", fmt.Sprintf("%s1", string(rune('A'+len(headers)-1))), style)
	}
	// freeze top row
	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: true, XSplit: 0, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})

	// data rows
	for r, row := range rows {
		for cidx, v := range row {
			cell, _ := excelize.CoordinatesToCellName(cidx+1, r+2)
			f.SetCellValue(sheet, cell, v)
		}
	}

	// instructions sheet
	if len(instructions) > 0 {
		instSheet := "說明"
		f.NewSheet(instSheet)
		for i, line := range instructions {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			f.SetCellValue(instSheet, cell, line)
		}
		if idx, err := f.GetSheetIndex(sheet); err == nil {
			f.SetActiveSheet(idx)
		}
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	return f.Write(c.Response().BodyWriter())
}

// DownloadOrdersImportTemplateExcel 下載訂單匯入模板（Excel）
func DownloadOrdersImportTemplateExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	withData := parseBoolQuery(c, "with_data")
	limit := parseIntQuery(c, "limit", 200, 2000)

	headers := []string{
		"order_number",
		"order_date",
		"status",
		"salesperson_employee_number",
		"product_code",
		"quantity",
		"unit_price",
		"notes",
	}
	instructions := []string{
		"用途：訂單匯入模板（每一行代表一個訂單明細；同一張單可有多行，order_number 重複）",
		"關聯規則：product_code -> products.code；salesperson_employee_number -> users.employee_number",
		"status 支援：draft / quotation / confirmed / processing / completed / cancelled（空白預設 confirmed）",
	}

	rows := make([][]interface{}, 0)
	if withData {
		type row struct {
			OrderNumber                string    `json:"order_number"`
			OrderDate                  time.Time `json:"order_date"`
			Status                     string    `json:"status"`
			SalespersonEmployeeNumber  string    `json:"salesperson_employee_number"`
			ProductCode                string    `json:"product_code"`
			Quantity                   float64   `json:"quantity"`
			UnitPrice                  float64   `json:"unit_price"`
			Notes                      string    `json:"notes"`
		}
		var data []row
		database.DB.Table("order_items oi").
			Select(`o.order_number as order_number,
					o.order_date as order_date,
					o.status as status,
					u.employee_number as salesperson_employee_number,
					p.code as product_code,
					oi.quantity as quantity,
					oi.unit_price as unit_price,
					oi.notes as notes`).
			Joins("JOIN orders o ON o.id = oi.order_id").
			Joins("LEFT JOIN users u ON u.id = o.salesperson_id").
			Joins("LEFT JOIN products p ON p.id = oi.product_id").
			Where("o.tenant_id = ?", tenantID).
			Order("o.created_at DESC, o.order_date DESC").
			Limit(limit).
			Scan(&data)

		for _, r := range data {
			rows = append(rows, []interface{}{
				r.OrderNumber,
				r.OrderDate.Format("2006-01-02"),
				r.Status,
				r.SalespersonEmployeeNumber,
				r.ProductCode,
				r.Quantity,
				r.UnitPrice,
				r.Notes,
			})
		}
	}

	filename := fmt.Sprintf("orders_import_template_%s.xlsx", map[bool]string{true: "with_data", false: "blank"}[withData])
	return writeTemplateExcel(c, filename, headers, rows, instructions)
}

// DownloadServiceOrdersImportTemplateExcel 下載服務單匯入模板（Excel）
func DownloadServiceOrdersImportTemplateExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	withData := parseBoolQuery(c, "with_data")
	limit := parseIntQuery(c, "limit", 200, 2000)

	headers := []string{
		"order_number",
		"service_date",
		"status",
		"customer_name",
		"salesperson_employee_number",
		"service_code",
		"staff_employee_number",
		"quantity",
		"unit_price",
		"notes",
	}
	instructions := []string{
		"用途：服務單匯入模板（每一行代表一個服務單明細；同一張單可有多行，order_number 重複）",
		"關聯規則：service_code -> services.code；staff_employee_number -> users.employee_number；salesperson_employee_number -> users.employee_number",
		"status 支援：draft / confirmed / processing / completed / cancelled（空白預設 confirmed）",
	}

	rows := make([][]interface{}, 0)
	if withData {
		type row struct {
			OrderNumber               string    `json:"order_number"`
			ServiceDate               time.Time `json:"service_date"`
			Status                    string    `json:"status"`
			CustomerName              string    `json:"customer_name"`
			SalespersonEmployeeNumber string    `json:"salesperson_employee_number"`
			ServiceCode               string    `json:"service_code"`
			StaffEmployeeNumber       string    `json:"staff_employee_number"`
			Quantity                  float64   `json:"quantity"`
			UnitPrice                 float64   `json:"unit_price"`
			Notes                     string    `json:"notes"`
		}
		var data []row
		database.DB.Table("service_order_items soi").
			Select(`so.order_number as order_number,
					so.service_date as service_date,
					so.status as status,
					c.name as customer_name,
					sp.employee_number as salesperson_employee_number,
					s.code as service_code,
					st.employee_number as staff_employee_number,
					soi.quantity as quantity,
					soi.unit_price as unit_price,
					soi.notes as notes`).
			Joins("JOIN service_orders so ON so.id = soi.service_order_id").
			Joins("LEFT JOIN customers c ON c.id = so.customer_id").
			Joins("LEFT JOIN users sp ON sp.id = so.salesperson_id").
			Joins("LEFT JOIN users st ON st.id = soi.staff_id").
			Joins("LEFT JOIN services s ON s.id = soi.service_id").
			Where("so.tenant_id = ?", tenantID).
			Order("so.created_at DESC, so.service_date DESC").
			Limit(limit).
			Scan(&data)

		for _, r := range data {
			rows = append(rows, []interface{}{
				r.OrderNumber,
				r.ServiceDate.Format("2006-01-02"),
				r.Status,
				r.CustomerName,
				r.SalespersonEmployeeNumber,
				r.ServiceCode,
				r.StaffEmployeeNumber,
				r.Quantity,
				r.UnitPrice,
				r.Notes,
			})
		}
	}

	filename := fmt.Sprintf("service_orders_import_template_%s.xlsx", map[bool]string{true: "with_data", false: "blank"}[withData])
	return writeTemplateExcel(c, filename, headers, rows, instructions)
}

// DownloadPurchaseOrdersImportTemplateExcel 下載採購單匯入模板（Excel）
func DownloadPurchaseOrdersImportTemplateExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	withData := parseBoolQuery(c, "with_data")
	limit := parseIntQuery(c, "limit", 200, 2000)

	headers := []string{
		"order_number",
		"order_date",
		"status",
		"supplier_code",
		"supplier_name",
		"product_code",
		"quantity",
		"unit_price",
		"notes",
	}
	instructions := []string{
		"用途：採購單匯入模板（每一行代表一個採購單明細；同一張單可有多行，order_number 重複）",
		"關聯規則：product_code -> products.code；supplier_code -> suppliers.code（或用 supplier_name 對應 suppliers.name）",
		"status 支援：draft / confirmed / cancelled / completed（空白預設 confirmed）",
	}

	rows := make([][]interface{}, 0)
	if withData {
		type row struct {
			OrderNumber   string    `json:"order_number"`
			OrderDate     time.Time `json:"order_date"`
			Status        string    `json:"status"`
			SupplierCode  string    `json:"supplier_code"`
			SupplierName  string    `json:"supplier_name"`
			ProductCode   string    `json:"product_code"`
			Quantity      int       `json:"quantity"`
			UnitPrice     float64   `json:"unit_price"`
			Notes         string    `json:"notes"`
		}
		var data []row
		database.DB.Table("purchase_order_items poi").
			Select(`po.order_number as order_number,
					po.order_date as order_date,
					po.status as status,
					sup.code as supplier_code,
					sup.name as supplier_name,
					p.code as product_code,
					poi.quantity as quantity,
					poi.unit_price as unit_price,
					poi.notes as notes`).
			Joins("JOIN purchase_orders po ON po.id = poi.purchase_order_id").
			Joins("LEFT JOIN suppliers sup ON sup.id = po.supplier_id").
			Joins("LEFT JOIN products p ON p.id = poi.product_id").
			Where("po.tenant_id = ?", tenantID).
			Order("po.created_at DESC, po.order_date DESC").
			Limit(limit).
			Scan(&data)

		for _, r := range data {
			rows = append(rows, []interface{}{
				r.OrderNumber,
				r.OrderDate.Format("2006-01-02"),
				r.Status,
				r.SupplierCode,
				r.SupplierName,
				r.ProductCode,
				r.Quantity,
				r.UnitPrice,
				r.Notes,
			})
		}
	}

	filename := fmt.Sprintf("purchase_orders_import_template_%s.xlsx", map[bool]string{true: "with_data", false: "blank"}[withData])
	return writeTemplateExcel(c, filename, headers, rows, instructions)
}


