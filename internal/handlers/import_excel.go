package handlers

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

type importResult struct {
	UpdatedOrders  int      `json:"updated_orders"`
	CreatedOrders  int      `json:"created_orders"`
	UpdatedItems   int      `json:"updated_items"`
	Warnings       []string `json:"warnings"`
}

func readUploadedExcel(c *fiber.Ctx) (*excelize.File, string, error) {
	file, err := c.FormFile("file")
	if err != nil {
		return nil, "", fmt.Errorf("No file uploaded")
	}
	fh, err := file.Open()
	if err != nil {
		return nil, "", fmt.Errorf("Failed to open uploaded file")
	}
	defer fh.Close()

	// excelize.OpenReader 會把內容讀入記憶體
	f, err := excelize.OpenReader(fh)
	if err != nil {
		return nil, "", fmt.Errorf("Invalid Excel file: %v", err)
	}

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		_ = f.Close()
		return nil, "", fmt.Errorf("Excel has no sheets")
	}
	return f, sheets[0], nil
}

func normalizeHeader(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func buildHeaderIndex(header []string) map[string]int {
	idx := map[string]int{}
	for i, h := range header {
		k := normalizeHeader(h)
		if k == "" {
			continue
		}
		// first wins
		if _, ok := idx[k]; !ok {
			idx[k] = i
		}
	}
	return idx
}

func cell(row []string, idx map[string]int, key string) string {
	i, ok := idx[normalizeHeader(key)]
	if !ok || i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	// 去掉千分位
	s = strings.ReplaceAll(s, ",", "")
	return strconv.ParseFloat(s, 64)
}

func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	s = strings.ReplaceAll(s, ",", "")
	i, err := strconv.Atoi(s)
	if err == nil {
		return i, nil
	}
	// 可能是 1.0
	f, ferr := strconv.ParseFloat(s, 64)
	if ferr != nil {
		return 0, err
	}
	return int(f + 0.5), nil
}

func parseDateForTenant(tenantID uuid.UUID, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return utils.NowInTenantTimezone(tenantID), nil
	}
	// 常見格式：yyyy-mm-dd
	if t, err := utils.ParseDateInTenantTimezone(tenantID, s); err == nil {
		return t, nil
	}
	// 其他格式 fallback
	layouts := []string{"2006/01/02", "02/01/2006", "2006-01-02 15:04:05"}
	for _, l := range layouts {
		if tt, err := time.Parse(l, s); err == nil {
			return tt, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date: %s", s)
}

func mustHave(idx map[string]int, keys ...string) error {
	for _, k := range keys {
		if _, ok := idx[normalizeHeader(k)]; !ok {
			return fmt.Errorf("Missing required column: %s", k)
		}
	}
	return nil
}

func readAllRows(f *excelize.File, sheet string) ([][]string, error) {
	rs, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	// 去掉全空行
	out := make([][]string, 0, len(rs))
	for _, r := range rs {
		allEmpty := true
		for _, c := range r {
			if strings.TrimSpace(c) != "" {
				allEmpty = false
				break
			}
		}
		if !allEmpty {
			out = append(out, r)
		}
	}
	return out, nil
}

// =============== Orders ===============

type orderImportLine struct {
	RowNo         int
	OrderNumber   string
	OrderDate     string
	Status        string
	SalesEmpNo    string
	ProductCode   string
	Qty           float64
	UnitPrice     float64
	Notes         string
}

func ImportOrdersFromExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	skipMissingProducts := parseBoolQuery(c, "skip_missing_products")

	f, sheet, err := readUploadedExcel(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	defer f.Close()

	rows, err := readAllRows(f, sheet)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Failed to read excel rows"})
	}
	if len(rows) < 2 {
		return c.Status(400).JSON(fiber.Map{"error": "Excel has no data rows"})
	}

	header := rows[0]
	idx := buildHeaderIndex(header)
	if err := mustHave(idx, "order_number", "product_code", "quantity", "unit_price"); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	linesByOrder := map[string][]orderImportLine{}
	orderHeaderSig := map[string]string{} // 用於檢查 header 一致性

	var warnings []string
	for i := 1; i < len(rows); i++ {
		r := rows[i]
		lineNo := i + 1
		on := cell(r, idx, "order_number")
		if on == "" {
			warnings = append(warnings, fmt.Sprintf("Row %d: empty order_number, skipped", lineNo))
			continue
		}
		qty, err := parseFloat(cell(r, idx, "quantity"))
		if err != nil || qty <= 0 {
			warnings = append(warnings, fmt.Sprintf("Row %d: invalid quantity, skipped", lineNo))
			continue
		}
		up, err := parseFloat(cell(r, idx, "unit_price"))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Row %d: invalid unit_price, skipped", lineNo))
			continue
		}
		pc := cell(r, idx, "product_code")
		if pc == "" {
			warnings = append(warnings, fmt.Sprintf("Row %d: empty product_code, skipped", lineNo))
			continue
		}

		od := cell(r, idx, "order_date")
		st := cell(r, idx, "status")
		se := cell(r, idx, "salesperson_employee_number")
		sig := strings.Join([]string{od, st, se}, "|")
		if prev, ok := orderHeaderSig[on]; ok && prev != sig {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Order %s header fields are inconsistent (row %d)", on, lineNo)})
		}
		orderHeaderSig[on] = sig

		linesByOrder[on] = append(linesByOrder[on], orderImportLine{
			RowNo:       lineNo,
			OrderNumber: on,
			OrderDate:   od,
			Status:      st,
			SalesEmpNo:  se,
			ProductCode: pc,
			Qty:         qty,
			UnitPrice:   up,
			Notes:       cell(r, idx, "notes"),
		})
	}

	orderNumbers := make([]string, 0, len(linesByOrder))
	for k := range linesByOrder {
		orderNumbers = append(orderNumbers, k)
	}
	sort.Strings(orderNumbers)

	res := importResult{Warnings: warnings}
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, on := range orderNumbers {
		lines := linesByOrder[on]
		if len(lines) == 0 {
			continue
		}
		first := lines[0]

		// resolve salesperson
		var salespersonID *uuid.UUID = nil
		if strings.TrimSpace(first.SalesEmpNo) != "" {
			var u models.User
			if err := tx.Where("tenant_id = ? AND employee_number = ?", tenantID, first.SalesEmpNo).First(&u).Error; err == nil {
				id := u.ID
				salespersonID = &id
			} else {
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Order %s: salesperson_employee_number not found: %s", on, first.SalesEmpNo)})
			}
		}

		od, err := parseDateForTenant(tenantID, first.OrderDate)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Order %s: invalid order_date: %s", on, first.OrderDate)})
		}
		status := strings.TrimSpace(first.Status)
		if status == "" {
			status = "confirmed"
		}

		// find or create order
		var order models.Order
		err = tx.Where("tenant_id = ? AND order_number = ?", tenantID, on).First(&order).Error
		now := utils.NowInTenantTimezone(tenantID)
		if err != nil {
			order = models.Order{
				TenantID:      tenantID,
				OrderNumber:   on,
				OrderDate:     od,
				Status:        status,
				SalespersonID: salespersonID,
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     now,
				UpdatedAt:     now,
				SourceType:    "erp",
				ExtraFields:   models.JSONB(map[string]interface{}{"import_source": "excel"}),
			}
			if err := tx.Create(&order).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create order: " + err.Error()})
			}
			res.CreatedOrders++
		} else {
			order.OrderDate = od
			order.Status = status
			order.SalespersonID = salespersonID
			order.UpdatedBy = &userID
			order.UpdatedAt = now
			if err := tx.Save(&order).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to update order: " + err.Error()})
			}
			res.UpdatedOrders++
		}

		// replace items
		if err := tx.Where("order_id = ?", order.ID).Delete(&models.OrderItem{}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to clear order items: " + err.Error()})
		}

		total := 0.0
		for _, ln := range lines {
			// resolve product
			var product models.Product
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, ln.ProductCode).First(&product).Error; err != nil {
				if skipMissingProducts {
					res.Warnings = append(res.Warnings, fmt.Sprintf("Row %d: product_code not found, skipped: %s", ln.RowNo, ln.ProductCode))
					continue
				}
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Row %d: product_code not found: %s", ln.RowNo, ln.ProductCode)})
			}
			pid := product.ID
			lineTotal := ln.Qty * ln.UnitPrice
			oi := models.OrderItem{
				TenantID:    tenantID,
				OrderID:     order.ID,
				ProductID:   &pid,
				Quantity:    ln.Qty,
				UnitPrice:   ln.UnitPrice,
				TotalPrice:  lineTotal,
				Notes:       ln.Notes,
				CreatedAt:   now,
				ExtraFields: models.JSONB(map[string]interface{}{"import_row": ln.RowNo}),
			}
			if err := tx.Create(&oi).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create order item: " + err.Error()})
			}
			res.UpdatedItems++
			total += lineTotal
		}

		// update total
		order.TotalAmount = total
		order.UpdatedBy = &userID
		order.UpdatedAt = now
		if err := tx.Save(&order).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update order total: " + err.Error()})
		}
	}

	tx.Commit()
	return c.JSON(res)
}

// =============== Service Orders ===============

type serviceOrderImportLine struct {
	RowNo       int
	OrderNumber string
	ServiceDate string
	Status      string
	Customer    string
	SalesEmpNo  string
	ServiceCode string
	StaffEmpNo  string
	Qty         float64
	UnitPrice   float64
	Notes       string
}

func ImportServiceOrdersFromExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}
	skipMissingServices := parseBoolQuery(c, "skip_missing_services")
	skipMissingStaff := parseBoolQuery(c, "skip_missing_staff")

	f, sheet, err := readUploadedExcel(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	defer f.Close()
	rows, err := readAllRows(f, sheet)
	if err != nil || len(rows) < 2 {
		return c.Status(400).JSON(fiber.Map{"error": "Excel has no data rows"})
	}
	idx := buildHeaderIndex(rows[0])
	if err := mustHave(idx, "order_number", "service_code", "quantity", "unit_price"); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	linesByOrder := map[string][]serviceOrderImportLine{}
	orderHeaderSig := map[string]string{}
	var warnings []string

	for i := 1; i < len(rows); i++ {
		r := rows[i]
		rowNo := i + 1
		on := cell(r, idx, "order_number")
		if on == "" {
			continue
		}
		qty, err := parseFloat(cell(r, idx, "quantity"))
		if err != nil || qty <= 0 {
			warnings = append(warnings, fmt.Sprintf("Row %d: invalid quantity, skipped", rowNo))
			continue
		}
		up, err := parseFloat(cell(r, idx, "unit_price"))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Row %d: invalid unit_price, skipped", rowNo))
			continue
		}
		sc := cell(r, idx, "service_code")
		if sc == "" {
			warnings = append(warnings, fmt.Sprintf("Row %d: empty service_code, skipped", rowNo))
			continue
		}

		sd := cell(r, idx, "service_date")
		st := cell(r, idx, "status")
		cn := cell(r, idx, "customer_name")
		se := cell(r, idx, "salesperson_employee_number")
		sig := strings.Join([]string{sd, st, cn, se}, "|")
		if prev, ok := orderHeaderSig[on]; ok && prev != sig {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("ServiceOrder %s header fields are inconsistent (row %d)", on, rowNo)})
		}
		orderHeaderSig[on] = sig

		linesByOrder[on] = append(linesByOrder[on], serviceOrderImportLine{
			RowNo:       rowNo,
			OrderNumber: on,
			ServiceDate: sd,
			Status:      st,
			Customer:    cn,
			SalesEmpNo:  se,
			ServiceCode: sc,
			StaffEmpNo:  cell(r, idx, "staff_employee_number"),
			Qty:         qty,
			UnitPrice:   up,
			Notes:       cell(r, idx, "notes"),
		})
	}

	orderNumbers := make([]string, 0, len(linesByOrder))
	for k := range linesByOrder {
		orderNumbers = append(orderNumbers, k)
	}
	sort.Strings(orderNumbers)

	res := importResult{Warnings: warnings}
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, on := range orderNumbers {
		lines := linesByOrder[on]
		if len(lines) == 0 {
			continue
		}
		first := lines[0]

		// customer (best-effort by name)
		var customerID *uuid.UUID = nil
		if strings.TrimSpace(first.Customer) != "" {
			var cust models.Customer
			if err := tx.Where("tenant_id = ? AND name = ?", tenantID, first.Customer).Order("created_at ASC").First(&cust).Error; err == nil {
				id := cust.ID
				customerID = &id
			} else {
				res.Warnings = append(res.Warnings, fmt.Sprintf("ServiceOrder %s: customer not found by name: %s", on, first.Customer))
			}
		}

		// salesperson
		var salespersonID *uuid.UUID = nil
		if strings.TrimSpace(first.SalesEmpNo) != "" {
			var u models.User
			if err := tx.Where("tenant_id = ? AND employee_number = ?", tenantID, first.SalesEmpNo).First(&u).Error; err == nil {
				id := u.ID
				salespersonID = &id
			} else {
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("ServiceOrder %s: salesperson_employee_number not found: %s", on, first.SalesEmpNo)})
			}
		}

		sd, err := parseDateForTenant(tenantID, first.ServiceDate)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("ServiceOrder %s: invalid service_date: %s", on, first.ServiceDate)})
		}
		status := strings.TrimSpace(first.Status)
		if status == "" {
			status = "confirmed"
		}

		// find or create service_order
		var so models.ServiceOrder
		err = tx.Where("tenant_id = ? AND order_number = ?", tenantID, on).First(&so).Error
		now := utils.NowInTenantTimezone(tenantID)
		if err != nil {
			so = models.ServiceOrder{
				TenantID:      tenantID,
				OrderNumber:   on,
				CustomerID:    customerID,
				ServiceDate:   sd,
				Status:        status,
				SalespersonID: salespersonID,
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     now,
				UpdatedAt:     now,
				ExtraFields:   models.JSONB(map[string]interface{}{"import_source": "excel"}),
			}
			if err := tx.Create(&so).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create service order: " + err.Error()})
			}
			res.CreatedOrders++
		} else {
			so.CustomerID = customerID
			so.ServiceDate = sd
			so.Status = status
			so.SalespersonID = salespersonID
			so.UpdatedBy = &userID
			so.UpdatedAt = now
			if err := tx.Save(&so).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to update service order: " + err.Error()})
			}
			res.UpdatedOrders++
		}

		// replace items
		if err := tx.Where("service_order_id = ?", so.ID).Delete(&models.ServiceOrderItem{}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to clear service order items: " + err.Error()})
		}

		total := 0.0
		for _, ln := range lines {
			// service
			var serviceID *uuid.UUID = nil
			var svc models.Service
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, ln.ServiceCode).First(&svc).Error; err == nil {
				id := svc.ID
				serviceID = &id
			} else if !skipMissingServices {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Row %d: service_code not found: %s", ln.RowNo, ln.ServiceCode)})
			} else {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Row %d: service_code not found, skipped: %s", ln.RowNo, ln.ServiceCode))
				continue
			}

			// staff
			var staffID *uuid.UUID = nil
			if strings.TrimSpace(ln.StaffEmpNo) != "" {
				var u models.User
				if err := tx.Where("tenant_id = ? AND employee_number = ?", tenantID, ln.StaffEmpNo).First(&u).Error; err == nil {
					id := u.ID
					staffID = &id
				} else if !skipMissingStaff {
					tx.Rollback()
					return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Row %d: staff_employee_number not found: %s", ln.RowNo, ln.StaffEmpNo)})
				} else {
					res.Warnings = append(res.Warnings, fmt.Sprintf("Row %d: staff_employee_number not found, skipped: %s", ln.RowNo, ln.StaffEmpNo))
					continue
				}
			}

			lineTotal := ln.Qty * ln.UnitPrice
			item := models.ServiceOrderItem{
				TenantID:       tenantID,
				ServiceOrderID: so.ID,
				ServiceID:      serviceID,
				StaffID:        staffID,
				Quantity:       ln.Qty,
				UnitPrice:      ln.UnitPrice,
				TotalPrice:     lineTotal,
				Notes:          ln.Notes,
				CreatedAt:      now,
				ExtraFields:    models.JSONB(map[string]interface{}{"import_row": ln.RowNo}),
			}
			if err := tx.Create(&item).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create service order item: " + err.Error()})
			}
			res.UpdatedItems++
			total += lineTotal
		}

		so.TotalAmount = total
		so.UpdatedBy = &userID
		so.UpdatedAt = now
		if err := tx.Save(&so).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update service order total: " + err.Error()})
		}
	}

	tx.Commit()
	return c.JSON(res)
}

// =============== Purchase Orders ===============

type purchaseOrderImportLine struct {
	RowNo       int
	OrderNumber string
	OrderDate   string
	Status      string
	SupplierCode string
	SupplierName string
	ProductCode string
	Qty         int
	UnitPrice   float64
	Notes       string
}

func ImportPurchaseOrdersFromExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}
	skipMissingProducts := parseBoolQuery(c, "skip_missing_products")

	f, sheet, err := readUploadedExcel(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	defer f.Close()
	rows, err := readAllRows(f, sheet)
	if err != nil || len(rows) < 2 {
		return c.Status(400).JSON(fiber.Map{"error": "Excel has no data rows"})
	}
	idx := buildHeaderIndex(rows[0])
	if err := mustHave(idx, "order_number", "product_code", "quantity", "unit_price"); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	linesByOrder := map[string][]purchaseOrderImportLine{}
	orderHeaderSig := map[string]string{}
	var warnings []string

	for i := 1; i < len(rows); i++ {
		r := rows[i]
		rowNo := i + 1
		on := cell(r, idx, "order_number")
		if on == "" {
			continue
		}
		qty, err := parseInt(cell(r, idx, "quantity"))
		if err != nil || qty <= 0 {
			warnings = append(warnings, fmt.Sprintf("Row %d: invalid quantity, skipped", rowNo))
			continue
		}
		up, err := parseFloat(cell(r, idx, "unit_price"))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Row %d: invalid unit_price, skipped", rowNo))
			continue
		}
		pc := cell(r, idx, "product_code")
		if pc == "" {
			warnings = append(warnings, fmt.Sprintf("Row %d: empty product_code, skipped", rowNo))
			continue
		}

		od := cell(r, idx, "order_date")
		st := cell(r, idx, "status")
		sc := cell(r, idx, "supplier_code")
		sn := cell(r, idx, "supplier_name")
		sig := strings.Join([]string{od, st, sc, sn}, "|")
		if prev, ok := orderHeaderSig[on]; ok && prev != sig {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("PurchaseOrder %s header fields are inconsistent (row %d)", on, rowNo)})
		}
		orderHeaderSig[on] = sig

		linesByOrder[on] = append(linesByOrder[on], purchaseOrderImportLine{
			RowNo:        rowNo,
			OrderNumber:  on,
			OrderDate:    od,
			Status:       st,
			SupplierCode: sc,
			SupplierName: sn,
			ProductCode:  pc,
			Qty:          qty,
			UnitPrice:    up,
			Notes:        cell(r, idx, "notes"),
		})
	}

	orderNumbers := make([]string, 0, len(linesByOrder))
	for k := range linesByOrder {
		orderNumbers = append(orderNumbers, k)
	}
	sort.Strings(orderNumbers)

	res := importResult{Warnings: warnings}
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, on := range orderNumbers {
		lines := linesByOrder[on]
		if len(lines) == 0 {
			continue
		}
		first := lines[0]

		// supplier (best-effort)
		var supplierID *uuid.UUID = nil
		if strings.TrimSpace(first.SupplierCode) != "" {
			var sup models.Supplier
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, first.SupplierCode).First(&sup).Error; err == nil {
				id := sup.ID
				supplierID = &id
			} else {
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("PurchaseOrder %s: supplier_code not found: %s", on, first.SupplierCode)})
			}
		} else if strings.TrimSpace(first.SupplierName) != "" {
			var sup models.Supplier
			if err := tx.Where("tenant_id = ? AND name = ?", tenantID, first.SupplierName).Order("created_at ASC").First(&sup).Error; err == nil {
				id := sup.ID
				supplierID = &id
			} else {
				res.Warnings = append(res.Warnings, fmt.Sprintf("PurchaseOrder %s: supplier not found by name: %s", on, first.SupplierName))
			}
		}

		od, err := parseDateForTenant(tenantID, first.OrderDate)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("PurchaseOrder %s: invalid order_date: %s", on, first.OrderDate)})
		}
		status := strings.TrimSpace(first.Status)
		if status == "" {
			status = "confirmed"
		}

		// find or create purchase_order
		var po models.PurchaseOrder
		err = tx.Where("tenant_id = ? AND order_number = ?", tenantID, on).First(&po).Error
		now := utils.NowInTenantTimezone(tenantID)
		if err != nil {
			po = models.PurchaseOrder{
				TenantID:     tenantID,
				SupplierID:   supplierID,
				OrderNumber:  on,
				OrderDate:    od,
				Status:       status,
				Notes:        "",
				ExtraFields:  models.JSONB(map[string]interface{}{"import_source": "excel"}),
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			if err := tx.Create(&po).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create purchase order: " + err.Error()})
			}
			res.CreatedOrders++
		} else {
			po.SupplierID = supplierID
			po.OrderDate = od
			po.Status = status
			po.UpdatedAt = now
			if err := tx.Save(&po).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to update purchase order: " + err.Error()})
			}
			res.UpdatedOrders++
		}

		// replace items
		if err := tx.Where("purchase_order_id = ?", po.ID).Delete(&models.PurchaseOrderItem{}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to clear purchase order items: " + err.Error()})
		}

		total := 0.0
		for _, ln := range lines {
			var product models.Product
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, ln.ProductCode).First(&product).Error; err != nil {
				if skipMissingProducts {
					res.Warnings = append(res.Warnings, fmt.Sprintf("Row %d: product_code not found, skipped: %s", ln.RowNo, ln.ProductCode))
					continue
				}
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Row %d: product_code not found: %s", ln.RowNo, ln.ProductCode)})
			}
			pid := product.ID
			lineTotal := float64(ln.Qty) * ln.UnitPrice
			it := models.PurchaseOrderItem{
				PurchaseOrderID: po.ID,
				ProductID:       &pid,
				Quantity:        ln.Qty,
				UnitPrice:       ln.UnitPrice,
				TotalAmount:     lineTotal,
				Notes:           ln.Notes,
				ExtraFields:     models.JSONB(map[string]interface{}{"import_row": ln.RowNo}),
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if err := tx.Create(&it).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create purchase order item: " + err.Error()})
			}
			res.UpdatedItems++
			total += lineTotal
		}

		po.TotalAmount = total
		po.FinalAmount = total
		po.UpdatedAt = now
		if err := tx.Save(&po).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update purchase order total: " + err.Error()})
		}
		_ = userID // keep parity (future audit fields)
	}

	tx.Commit()
	return c.JSON(res)
}


