package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/phpdave11/gofpdf"
)

// GenerateInvoicePDF 為付款記錄生成發票 PDF
func GenerateInvoicePDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	orderID, err := uuid.Parse(c.Params("orderId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order ID"})
	}

	recordIndex, err := strconv.Atoi(c.Params("index"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid record index"})
	}

	// 獲取訂單
	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", orderID, tenantID).
		Preload("Customer").Preload("OrderItems.Product").First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}

	// 從 income 表獲取付款記錄（不再從 ExtraFields 讀取）
	var incomes []models.Income
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "order", orderID).Find(&incomes)

	var paymentRecords []map[string]interface{}
	if len(incomes) > 0 {
		paymentRecords = make([]map[string]interface{}, len(incomes))
		for i, inc := range incomes {
			ef := map[string]interface{}{}
			if inc.ExtraFields != nil {
				ef = map[string]interface{}(inc.ExtraFields)
			}
			paymentRecords[i] = map[string]interface{}{
				"income_id":         inc.ID.String(),
				"payment_date":      inc.IncomeDate.Format("2006-01-02"),
				"payment_method":    inc.PaymentMethod,
				"payment_method_id": ef["payment_method_id"],
				"bank_account_id":   inc.BankAccountID,
				"amount":            inc.Amount,
				"invoice_number":    ef["invoice_number"],
				"reference_number":  ef["reference_number"],
				"notes":             inc.Notes,
			}
		}
	}

	if recordIndex < 0 || recordIndex >= len(paymentRecords) {
		return c.Status(404).JSON(fiber.Map{"error": "Payment record not found"})
	}

	paymentRecord := paymentRecords[recordIndex]

	// 獲取發票設定
	var invoiceSettings models.DocumentSettings
	if err := database.DB.Where("tenant_id = ? AND document_type = ?", tenantID, "invoice").First(&invoiceSettings).Error; err != nil {
		// 使用默認值
		invoiceSettings = models.DocumentSettings{
			Terms: "",
			Notes: "",
		}
	}

	// 獲取發票編號（應該已經在創建付款記錄時自動生成）
	invoiceNumber := ""
	if invoiceNum, ok := paymentRecord["invoice_number"].(string); ok && invoiceNum != "" {
		invoiceNumber = invoiceNum
	} else {
		// 如果沒有，則自動生成（這種情況不應該發生，因為創建時已經生成）
		// 但為了向後兼容，保留此邏輯
		today := time.Now().Format("20060102")
		datePrefix := "INV-" + today + "-"
		var count int64
		database.DB.Model(&models.Invoice{}).Where("tenant_id = ? AND invoice_number LIKE ?", tenantID, datePrefix+"%").Count(&count)
		invoiceNumber = datePrefix + fmt.Sprintf("%03d", count+1)

		// 保存發票編號到 income 記錄的 ExtraFields
		if recordIndex < len(incomes) {
			income := incomes[recordIndex]
			ef := map[string]interface{}{}
			if income.ExtraFields != nil {
				ef = map[string]interface{}(income.ExtraFields)
			}
			ef["invoice_number"] = invoiceNumber
			income.ExtraFields = models.JSONB(ef)
			database.DB.Save(&income)
		}
	}

	// 獲取企業信息
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	// 使用 gofpdf 構建完整的發票 PDF
	pdfBytes, err := buildInvoicePDF(order, paymentRecord, invoiceNumber, invoiceSettings, enterprise, false)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate invoice PDF: " + err.Error()})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=invoice_%s.pdf", invoiceNumber))
	return c.Send(pdfBytes)
}

// GenerateQuotationPDF 為報價單生成 PDF（使用發票版面但無付款記錄）
func GenerateQuotationPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	orderID, err := uuid.Parse(c.Params("orderId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order ID"})
	}

	// 取得訂單（報價單）
	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", orderID, tenantID).
		Preload("Customer").Preload("OrderItems.Product").First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}
	if order.Status != "quotation" {
		return c.Status(400).JSON(fiber.Map{"error": "Only quotations can be exported here"})
	}

	// 構造一個虛擬的付款記錄，金額為訂單總額
	paymentRecord := map[string]interface{}{
		"payment_date":     time.Now().Format("2006-01-02"),
		"payment_method":   "other",
		"reference_number": order.OrderNumber,
		"amount":           order.TotalAmount,
	}

	// 發票/條款設定
	var invoiceSettings models.DocumentSettings
	if err := database.DB.Where("tenant_id = ? AND document_type = ?", tenantID, "invoice").First(&invoiceSettings).Error; err != nil {
		invoiceSettings = models.DocumentSettings{Terms: "", Notes: ""}
	}

	// 企業資訊
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	// 發票號 = 報價單號
	invoiceNumber := order.OrderNumber

	pdfBytes, err := buildInvoicePDF(order, paymentRecord, invoiceNumber, invoiceSettings, enterprise, true)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate quotation PDF: " + err.Error()})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=quotation_%s.pdf", order.OrderNumber))
	return c.Send(pdfBytes)
}

// GenerateServiceOrderPaymentPDF 為服務單付款記錄生成發票 PDF（沿用訂單發票版面）
func GenerateServiceOrderPaymentPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	serviceOrderID, err := uuid.Parse(c.Params("serviceOrderId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid service order ID"})
	}
	recordIndex, err := strconv.Atoi(c.Params("index"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid record index"})
	}

	// 取得服務單
	var so models.ServiceOrder
	if err := database.DB.Where("id = ? AND tenant_id = ?", serviceOrderID, tenantID).
		Preload("Customer").
		Preload("ServiceOrderItems.Service").
		First(&so).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service order not found"})
	}

	// 從 income 取得付款記錄
	var incomes []models.Income
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "service_order", so.ID).Find(&incomes)
	if recordIndex < 0 || recordIndex >= len(incomes) {
		return c.Status(404).JSON(fiber.Map{"error": "Payment record not found"})
	}
	inc := incomes[recordIndex]
	ef := map[string]interface{}{}
	if inc.ExtraFields != nil {
		ef = map[string]interface{}(inc.ExtraFields)
	}

	// 生成/取得發票號
	invoiceNumber := ""
	if inv, ok := ef["invoice_number"].(string); ok && inv != "" {
		invoiceNumber = inv
	} else {
		today := time.Now().Format("20060102")
		datePrefix := "SINV-" + today + "-"
		var count int64
		database.DB.Model(&models.Invoice{}).Where("tenant_id = ? AND invoice_number LIKE ?", tenantID, datePrefix+"%").Count(&count)
		invoiceNumber = datePrefix + fmt.Sprintf("%03d", count+1)
		ef["invoice_number"] = invoiceNumber
		inc.ExtraFields = models.JSONB(ef)
		database.DB.Save(&inc)
	}

	// 構造 paymentRecord 供版面使用
	paymentRecord := map[string]interface{}{
		"payment_date":      inc.IncomeDate.Format("2006-01-02"),
		"payment_method":    inc.PaymentMethod,
		"payment_method_id": ef["payment_method_id"],
		"bank_account_id":   inc.BankAccountID,
		"amount":            inc.Amount,
		"invoice_number":    ef["invoice_number"],
		"reference_number":  ef["reference_number"],
		"notes":             inc.Notes,
	}

	// 發票/條款設定
	var invoiceSettings models.DocumentSettings
	if err := database.DB.Where("tenant_id = ? AND document_type = ?", tenantID, "invoice").First(&invoiceSettings).Error; err != nil {
		invoiceSettings = models.DocumentSettings{Terms: "", Notes: ""}
	}

	// 企業資訊
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	// 建立 PDF（服務明細版）
	pdfBytes, err := buildServiceInvoicePDF(so, paymentRecord, invoiceNumber, invoiceSettings, enterprise)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate service order invoice PDF: " + err.Error()})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=service_invoice_%s.pdf", invoiceNumber))
	return c.Send(pdfBytes)
}

// GenerateShippingNotePDF 為出貨記錄生成發貨單 PDF
func GenerateShippingNotePDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	orderID, err := uuid.Parse(c.Params("orderId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order ID"})
	}

	recordIndex, err := strconv.Atoi(c.Params("index"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid record index"})
	}

	// 獲取訂單
	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", orderID, tenantID).
		Preload("Customer").Preload("OrderItems.Product").First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}

	// 從 ExtraFields 中提取 shipping_notes（新結構）或 shipping_records（舊結構）
	var shippingNotes []map[string]interface{}
	if order.ExtraFields != nil {
		fields := map[string]interface{}(order.ExtraFields)
		// 優先使用新的 shipping_notes 結構
		if notes, exists := fields["shipping_notes"]; exists {
			if notesList, ok := notes.([]interface{}); ok {
				for _, n := range notesList {
					if note, ok := n.(map[string]interface{}); ok {
						shippingNotes = append(shippingNotes, note)
					}
				}
			}
		} else if records, exists := fields["shipping_records"]; exists {
			// 向後兼容舊的 shipping_records 結構
			if recordsList, ok := records.([]interface{}); ok {
				for _, r := range recordsList {
					if record, ok := r.(map[string]interface{}); ok {
						shippingNotes = append(shippingNotes, record)
					}
				}
			}
		}
	}

	if recordIndex < 0 || recordIndex >= len(shippingNotes) {
		return c.Status(404).JSON(fiber.Map{"error": "Shipping note not found"})
	}

	shippingNote := shippingNotes[recordIndex]

	// 獲取發貨單設定
	var shippingSettings models.DocumentSettings
	if err := database.DB.Where("tenant_id = ? AND document_type = ?", tenantID, "shipping_note").First(&shippingSettings).Error; err != nil {
		// 使用默認值
		shippingSettings = models.DocumentSettings{
			Terms: "",
			Notes: "",
		}
	}

	// 獲取發貨單編號（應該已經在創建出貨記錄時自動生成）
	shippingNumber := ""
	if shippingNum, ok := shippingNote["shipping_number"].(string); ok && shippingNum != "" {
		shippingNumber = shippingNum
	} else {
		// 如果沒有，則自動生成（這種情況不應該發生，因為創建時已經生成）
		// 但為了向後兼容，保留此邏輯
		today := time.Now().Format("20060102")
		datePrefix := "SHIP-" + today + "-"
		var count int64
		// 查詢現有的發貨單編號（從 ExtraFields 中）
		var orders []models.Order
		database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").Find(&orders)
		count = int64(len(orders))
		shippingNumber = datePrefix + fmt.Sprintf("%03d", count+1)

		// 保存發貨單編號到出貨記錄
		shippingNote["shipping_number"] = shippingNumber
		if order.ExtraFields != nil {
			fields := map[string]interface{}(order.ExtraFields)
			// 優先更新 shipping_notes，如果不存在則更新 shipping_records
			if _, exists := fields["shipping_notes"]; exists {
				fields["shipping_notes"] = shippingNotes
			} else {
				fields["shipping_records"] = shippingNotes
			}
			order.ExtraFields = models.JSONB(fields)
			database.DB.Save(&order)
		}
	}

	// 獲取企業信息
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	// 生成發貨單 PDF
	// 如果 shippingNote 有 items，使用 items；否則使用整個訂單的 OrderItems
	headers := []string{"商品名稱", "數量"}
	rows := make([][]string, 0)

	if items, exists := shippingNote["items"].([]interface{}); exists && len(items) > 0 {
		// 使用 shippingNote 中的 items
		for _, itemInterface := range items {
			if item, ok := itemInterface.(map[string]interface{}); ok {
				productID, _ := item["product_id"].(string)
				quantity := 0.0
				if qty, ok := item["quantity"].(float64); ok {
					quantity = qty
				}

				// 查找產品名稱
				productName := "商品"
				if productID != "" {
					var product models.Product
					if err := database.DB.Where("id = ? AND tenant_id = ?", productID, tenantID).First(&product).Error; err == nil {
						productName = product.Name
					}
				}

				rows = append(rows, []string{productName, fmt.Sprintf("%.2f", quantity)})
			}
		}
	} else {
		// 向後兼容：使用整個訂單的 OrderItems
		for _, item := range order.OrderItems {
			name := "商品"
			if item.Product != nil {
				name = item.Product.Name
			}
			rows = append(rows, []string{name, fmt.Sprintf("%.2f", float64(item.Quantity))})
		}
	}

	pdfBytes, err := buildShippingNotePDF(order, shippingNote, shippingNumber, shippingSettings, enterprise, headers, rows)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate shipping note PDF: " + err.Error()})
	}
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=shipping_note_%s.pdf", shippingNumber))
	return c.Send(pdfBytes)
}

// getEnterpriseName 獲取企業名稱
func getEnterpriseName(enterprise models.Enterprise) string {
	if enterprise.Name != "" {
		return enterprise.Name
	}
	return "企業名稱"
}

// getEnterpriseAddress 獲取企業地址
func getEnterpriseAddress(enterprise models.Enterprise) string {
	if enterprise.Address != nil && *enterprise.Address != "" {
		return *enterprise.Address
	}
	return ""
}

// getEnterpriseLogoHTML 生成企業 logo 的 HTML img 標籤（放在公司名稱上方）
func getEnterpriseLogoHTML(enterprise models.Enterprise) string {
	if enterprise.LogoURL == nil {
		return ""
	}
	raw := strings.TrimSpace(*enterprise.LogoURL)
	if raw == "" || strings.Contains(raw, "logo3.png") {
		return ""
	}
	return fmt.Sprintf(`<img src="%s" alt="%s" style="max-height:80px;max-width:240px;display:block;margin-bottom:8px;" />`, raw, getEnterpriseName(enterprise))
}

func resolveEnterpriseLogoPath(enterprise models.Enterprise) (string, func()) {
	if enterprise.LogoURL == nil {
		log.Printf("[PDF-Logo] enterprise %s: LogoURL is nil, skipping", enterprise.Name)
		return "", nil
	}

	raw := strings.TrimSpace(*enterprise.LogoURL)
	if raw == "" {
		log.Printf("[PDF-Logo] enterprise %s: LogoURL is empty, skipping", enterprise.Name)
		return "", nil
	}

	if strings.Contains(raw, "logo3.png") {
		return "", nil
	}

	log.Printf("[PDF-Logo] enterprise %s: attempting to resolve logo_url=%q", enterprise.Name, raw)

	// 嘗試從 HTTP/HTTPS URL 獲取
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		if p, cleanup := fetchRemoteLogo(enterprise.Name, raw); p != "" {
			return p, cleanup
		}
		return "", nil
	}

	// 本地路徑解析：嘗試多個可能的路徑
	candidates := buildLogoCandidatePaths(raw)
	cwd, _ := os.Getwd()
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if !isSupportedImageExt(candidate) {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			log.Printf("[PDF-Logo] enterprise %s: resolved logo at %s (raw: %s, cwd: %s)", enterprise.Name, candidate, raw, cwd)
			return candidate, nil
		}
	}

	// 所有本地路徑都找不到，嘗試透過 localhost HTTP fallback 獲取
	// 這可處理 CWD 不在 vwork 目錄等邊緣情況
	if strings.HasPrefix(raw, "/") {
		port := os.Getenv("PORT")
		if port == "" {
			port = "3001"
		}
		localURL := fmt.Sprintf("http://127.0.0.1:%s%s", port, raw)
		log.Printf("[PDF-Logo] enterprise %s: local file not found, trying HTTP fallback: %s", enterprise.Name, localURL)
		if p, cleanup := fetchRemoteLogo(enterprise.Name, localURL); p != "" {
			return p, cleanup
		}
	}

	log.Printf("[PDF-Logo] enterprise %s: logo file not found after all attempts (raw: %s, candidates: %v, cwd: %s)", enterprise.Name, raw, candidates, cwd)
	return "", nil
}

// buildLogoCandidatePaths 根據 raw logo_url 構建所有可能的本地路徑
func buildLogoCandidatePaths(raw string) []string {
	var paths []string

	if filepath.IsAbs(raw) {
		paths = append(paths, raw)
		return paths
	}

	if strings.HasPrefix(raw, "/") {
		trimmed := strings.TrimPrefix(raw, "/")
		// 標準路徑：web/ + uploads/...
		paths = append(paths, filepath.Join("web", trimmed))
		// 也嘗試 ./web/...
		paths = append(paths, filepath.Join(".", "web", trimmed))
		// 直接嘗試去掉 / 的路徑
		paths = append(paths, trimmed)
	} else if strings.HasPrefix(raw, "web/") || strings.HasPrefix(raw, "web\\") {
		paths = append(paths, raw)
		paths = append(paths, filepath.Join(".", raw))
	} else {
		paths = append(paths, filepath.Join("web", raw))
		paths = append(paths, filepath.Join(".", "web", raw))
		paths = append(paths, raw)
	}

	return paths
}

// fetchRemoteLogo 透過 HTTP GET 獲取遠端 Logo 並存為臨時檔
func fetchRemoteLogo(enterpriseName, url string) (string, func()) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("[PDF-Logo] enterprise %s: HTTP GET %s failed: %v", enterpriseName, url, err)
		return "", nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[PDF-Logo] enterprise %s: HTTP GET %s returned status %d", enterpriseName, url, resp.StatusCode)
		return "", nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[PDF-Logo] enterprise %s: failed to read response body from %s: %v", enterpriseName, url, err)
		return "", nil
	}
	ext := imageExtFromContentType(http.DetectContentType(data))
	if ext == "" {
		log.Printf("[PDF-Logo] enterprise %s: unsupported image type from %s", enterpriseName, url)
		return "", nil
	}
	tmpFile, err := os.CreateTemp("", "vwork-logo-*"+ext)
	if err != nil {
		log.Printf("[PDF-Logo] enterprise %s: failed to create temp file: %v", enterpriseName, err)
		return "", nil
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		log.Printf("[PDF-Logo] enterprise %s: failed to write temp file from %s: %v", enterpriseName, url, err)
		return "", nil
	}
	_ = tmpFile.Close()
	log.Printf("[PDF-Logo] enterprise %s: fetched logo from %s to %s", enterpriseName, url, tmpFile.Name())
	return tmpFile.Name(), func() { _ = os.Remove(tmpFile.Name()) }
}

func imageExtFromContentType(contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "jpeg") || strings.Contains(ct, "jpg"):
		return ".jpg"
	case strings.Contains(ct, "gif"):
		return ".gif"
	default:
		return ""
	}
}

func isSupportedImageExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	default:
		return false
	}
}

func drawEnterpriseLogoOrName(pdf *gofpdf.Fpdf, enterprise models.Enterprise, fontName string, hasBold bool, x, y, usableW, lineH float64) (float64, func()) {
	return drawDocumentLogo(pdf, enterprise, nil, fontName, hasBold, x, y, usableW, lineH)
}

// drawDocumentLogo 繪製文件 Logo。優先使用 styleSettings 中的文件 Logo，否則 fallback 到企業 Logo。
func drawDocumentLogo(pdf *gofpdf.Fpdf, enterprise models.Enterprise, styleSettings *models.DocumentSettings, fontName string, hasBold bool, x, y, usableW, lineH float64) (float64, func()) {
	// 決定 logo 路徑：優先文件 Logo，再 fallback 企業 Logo
	var logoPath string
	var cleanup func()

	if styleSettings != nil && styleSettings.LogoURL != nil && *styleSettings.LogoURL != "" {
		// 用文件專屬 Logo（構造一個臨時 Enterprise 來複用 resolveEnterpriseLogoPath）
		tmpEnt := models.Enterprise{Name: enterprise.Name, LogoURL: styleSettings.LogoURL}
		logoPath, cleanup = resolveEnterpriseLogoPath(tmpEnt)
	}
	if logoPath == "" {
		logoPath, cleanup = resolveEnterpriseLogoPath(enterprise)
	}

	if logoPath != "" {
		// Logo 大小：優先使用 styleSettings，否則使用預設
		logoMaxW := usableW * 0.35
		logoMaxH := lineH * 2.5
		if logoMaxW < 80 {
			logoMaxW = 80
		}
		if logoMaxH < 35 {
			logoMaxH = 35
		}
		if styleSettings != nil && styleSettings.LogoWidth > 0 {
			logoMaxW = styleSettings.LogoWidth
		}
		if styleSettings != nil && styleSettings.LogoHeight > 0 {
			logoMaxH = styleSettings.LogoHeight
		}
		if h, err := drawLogo(pdf, logoPath, x, y, logoMaxW, logoMaxH); err == nil && h > 0 {
			return h, cleanup
		}
		if cleanup != nil {
			cleanup()
		}
	}

	if hasBold {
		pdf.SetFont(fontName, "B", 12)
	} else {
		pdf.SetFont(fontName, "", 12)
	}
	pdf.SetXY(x, y)
	pdf.CellFormat(usableW, lineH, getEnterpriseName(enterprise), "", 1, "L", false, 0, "")
	return lineH, nil
}

func drawLogo(pdf *gofpdf.Fpdf, logoPath string, x, y, maxW, maxH float64) (float64, error) {
	opts := gofpdf.ImageOptions{ReadDpi: true}
	info := pdf.RegisterImageOptions(logoPath, opts)
	if info == nil {
		return 0, fmt.Errorf("failed to register logo")
	}
	w, h := info.Extent()
	if w <= 0 || h <= 0 {
		return 0, fmt.Errorf("invalid logo size")
	}
	scale := math.Min(maxW/w, maxH/h)
	if scale > 1 {
		scale = 1
	}
	dw := w * scale
	dh := h * scale
	pdf.ImageOptions(logoPath, x, y, dw, dh, false, opts, 0, "")
	return dh, nil
}

// generateInvoiceHTML 生成發票 HTML
func generateInvoiceHTML(order models.Order, paymentRecord map[string]interface{}, invoiceNumber string, settings models.DocumentSettings, enterprise models.Enterprise) string {
	customerName := "散客"
	if order.Customer != nil {
		customerName = order.Customer.Name
	}

	contactName := order.ContactName
	if contactName == "" && order.Customer != nil {
		contactName = order.Customer.Name
	}

	contactAddress := order.ContactAddress
	if contactAddress == "" && order.Customer != nil {
		contactAddress = order.Customer.Address
	}

	contactPhone := order.ContactPhone
	if contactPhone == "" && order.Customer != nil {
		contactPhone = order.Customer.Phone
	}

	paymentDate := ""
	if date, ok := paymentRecord["payment_date"].(string); ok {
		paymentDate = date
	}

	paymentMethod := ""
	if method, ok := paymentRecord["payment_method"].(string); ok {
		methodMap := map[string]string{
			"cash":          "現金",
			"credit_card":   "信用卡",
			"bank_transfer": "銀行轉帳",
			"check":         "支票",
			"other":         "其他",
		}
		paymentMethod = methodMap[method]
	}

	amount := 0.0
	if amt, ok := paymentRecord["amount"].(float64); ok {
		amount = amt
	}

	referenceNumber := ""
	if ref, ok := paymentRecord["reference_number"].(string); ok {
		referenceNumber = ref
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>發票 - %s</title>
	<style>
		body { font-family: Arial, "Microsoft YaHei", sans-serif; margin: 20px; }
		.header { text-align: center; margin-bottom: 30px; }
		.company-info { margin-bottom: 20px; }
		.invoice-info { margin-bottom: 20px; }
		.customer-info { margin-bottom: 20px; }
		table { width: 100%%; border-collapse: collapse; margin-bottom: 20px; }
		th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
		th { background-color: #f2f2f2; }
		.total { text-align: right; font-weight: bold; font-size: 1.2em; }
		.terms { margin-top: 30px; padding-top: 20px; border-top: 2px solid #333; }
		.notes { margin-top: 20px; }
		@media print {
			body { margin: 0; }
		}
	</style>
</head>
<body>
	<div class="header">
		<h1>發票</h1>
		<h2>INVOICE</h2>
	</div>
	
	<div class="company-info">
		%s
		<h3>%s</h3>
		%s
	</div>
	
	<div class="invoice-info">
		<p><strong>發票編號：</strong>%s</p>
		<p><strong>發票日期：</strong>%s</p>
		<p><strong>訂單編號：</strong>%s</p>
	</div>
	
	<div class="customer-info">
		<h3>客戶信息</h3>
		<p><strong>客戶名稱：</strong>%s</p>
		<p><strong>聯絡人：</strong>%s</p>
		<p><strong>地址：</strong>%s</p>
		<p><strong>電話：</strong>%s</p>
	</div>
	
	<table>
		<thead>
			<tr>
				<th>商品名稱</th>
				<th>數量</th>
				<th>單價</th>
				<th>小計</th>
			</tr>
		</thead>
		<tbody>`, invoiceNumber, getEnterpriseLogoHTML(enterprise), getEnterpriseName(enterprise), getEnterpriseAddress(enterprise), invoiceNumber, paymentDate, order.OrderNumber, customerName, contactName, contactAddress, contactPhone)

	for _, item := range order.OrderItems {
		productName := "商品"
		if item.Product != nil {
			productName = item.Product.Name
		}
		html += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%.2f</td>
				<td>$%.2f</td>
				<td>$%.2f</td>
			</tr>`, productName, item.Quantity, item.UnitPrice, item.TotalPrice)
	}

	html += fmt.Sprintf(`
		</tbody>
		<tfoot>
			<tr>
				<td colspan="3" class="total">付款方式：</td>
				<td>%s</td>
			</tr>
			<tr>
				<td colspan="3" class="total">參考號碼：</td>
				<td>%s</td>
			</tr>
			<tr>
				<td colspan="3" class="total">應付總額：</td>
				<td>$%.2f</td>
			</tr>
		</tfoot>
	</table>
	
	<div class="terms">
		<h3>條款</h3>
		<div>%s</div>
	</div>
	
	<div class="notes">
		<h3>備註</h3>
		<div>%s</div>
	</div>
</body>
</html>`, paymentMethod, referenceNumber, amount, settings.Terms, settings.Notes)

	return html
}

// GeneratePurchaseOrderPaymentPDF 為採購單付款記錄生成付款單 PDF
func GeneratePurchaseOrderPaymentPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	poID, err := uuid.Parse(c.Params("purchaseOrderId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid purchase order ID"})
	}

	recordIndex, err := strconv.Atoi(c.Params("index"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid record index"})
	}

	// 獲取採購單
	var po models.PurchaseOrder
	if err := database.DB.Where("id = ? AND tenant_id = ?", poID, tenantID).
		Preload("Supplier").Preload("PurchaseOrderItems.Product").First(&po).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order not found"})
	}

	// 從 ExtraFields 中提取 payment_records
	var paymentRecords []map[string]interface{}
	if po.ExtraFields != nil {
		fields := map[string]interface{}(po.ExtraFields)
		if records, exists := fields["payment_records"]; exists {
			if recordsList, ok := records.([]interface{}); ok {
				for _, r := range recordsList {
					if record, ok := r.(map[string]interface{}); ok {
						paymentRecords = append(paymentRecords, record)
					}
				}
			}
		}
	}

	if recordIndex < 0 || recordIndex >= len(paymentRecords) {
		return c.Status(404).JSON(fiber.Map{"error": "Payment record not found"})
	}

	paymentRecord := paymentRecords[recordIndex]

	// 獲取企業信息
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	// 使用發票版面生成「採購支出單」
	paymentDate, _ := paymentRecord["payment_date"].(string)
	paymentMethod, _ := paymentRecord["payment_method"].(string)
	referenceNumber, _ := paymentRecord["reference_number"].(string)
	invoiceNumber, _ := paymentRecord["invoice_number"].(string)
	amount := 0.0
	if amt, ok := paymentRecord["amount"].(float64); ok {
		amount = amt
	}

	paymentRecord["payment_date"] = paymentDate
	paymentRecord["payment_method"] = paymentMethod
	paymentRecord["reference_number"] = referenceNumber
	paymentRecord["amount"] = amount

	if invoiceNumber == "" {
		invoiceNumber = fmt.Sprintf("PO-PAY-%s-%02d", time.Now().Format("20060102"), recordIndex+1)
	}
	invoiceSettings := models.DocumentSettings{Terms: "", Notes: ""}
	pdfBytes, err := buildPurchasePaymentPDF(po, paymentRecord, invoiceNumber, invoiceSettings, enterprise)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate purchase payment PDF: " + err.Error()})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=purchase_expense_%s_%d.pdf", po.OrderNumber, recordIndex))
	return c.Send(pdfBytes)
}

// generatePurchasePaymentHTML 生成採購單付款單 HTML
func generatePurchasePaymentHTML(po models.PurchaseOrder, paymentRecord map[string]interface{}, enterprise models.Enterprise) string {
	supplierName := "供應商"
	if po.Supplier != nil {
		supplierName = po.Supplier.Name
	}

	paymentDate := ""
	if date, ok := paymentRecord["payment_date"].(string); ok {
		paymentDate = date
	}

	paymentMethod := ""
	if method, ok := paymentRecord["payment_method"].(string); ok {
		methodMap := map[string]string{
			"cash":          "現金",
			"credit_card":   "信用卡",
			"bank_transfer": "銀行轉帳",
			"check":         "支票",
			"other":         "其他",
		}
		paymentMethod = methodMap[method]
	}

	amount := 0.0
	if amt, ok := paymentRecord["amount"].(float64); ok {
		amount = amt
	}

	referenceNumber := ""
	if ref, ok := paymentRecord["reference_number"].(string); ok {
		referenceNumber = ref
	}

	invoiceNumber := ""
	if inv, ok := paymentRecord["invoice_number"].(string); ok {
		invoiceNumber = inv
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>付款單 - %s</title>
	<style>
		body { font-family: Arial, "Microsoft YaHei", sans-serif; margin: 20px; }
		.header { text-align: center; margin-bottom: 30px; }
		.company-info { margin-bottom: 20px; }
		.payment-info { margin-bottom: 20px; }
		.supplier-info { margin-bottom: 20px; }
		table { width: 100%%; border-collapse: collapse; margin-bottom: 20px; }
		th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
		th { background-color: #f2f2f2; }
		.total { text-align: right; font-weight: bold; font-size: 1.2em; }
		@media print {
			body { margin: 0; }
		}
	</style>
</head>
<body>
	<div class="header">
		<h1>付款單</h1>
		<h2>PAYMENT VOUCHER</h2>
	</div>
	
	<div class="company-info">
		%s
		<h3>%s</h3>
		%s
	</div>
	
	<div class="payment-info">
		<p><strong>付款日期：</strong>%s</p>
		<p><strong>採購單編號：</strong>%s</p>
		<p><strong>發票號碼：</strong>%s</p>
		<p><strong>參考號碼：</strong>%s</p>
	</div>
	
	<div class="supplier-info">
		<h3>供應商信息</h3>
		<p><strong>供應商名稱：</strong>%s</p>
	</div>
	
	<table>
		<thead>
			<tr>
				<th>商品名稱</th>
				<th>數量</th>
				<th>單價</th>
				<th>小計</th>
			</tr>
		</thead>
		<tbody>`, po.OrderNumber, getEnterpriseLogoHTML(enterprise), getEnterpriseName(enterprise), getEnterpriseAddress(enterprise), paymentDate, po.OrderNumber, invoiceNumber, referenceNumber, supplierName)

	for _, item := range po.PurchaseOrderItems {
		productName := "商品"
		if item.Product != nil {
			productName = item.Product.Name
		}
		html += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%d</td>
				<td>$%.2f</td>
				<td>$%.2f</td>
			</tr>`, productName, item.Quantity, item.UnitPrice, item.TotalAmount)
	}

	html += fmt.Sprintf(`
		</tbody>
		<tfoot>
			<tr>
				<td colspan="3" class="total">付款方式：</td>
				<td>%s</td>
			</tr>
			<tr>
				<td colspan="3" class="total">付款金額：</td>
				<td>$%.2f</td>
			</tr>
		</tfoot>
	</table>
</body>
</html>`, paymentMethod, amount)

	return html
}

// generateShippingNoteHTML 生成發貨單 HTML
func generateShippingNoteHTML(order models.Order, shippingRecord map[string]interface{}, shippingNumber string, settings models.DocumentSettings, enterprise models.Enterprise) string {
	contactName := order.ContactName
	if contactName == "" && order.Customer != nil {
		contactName = order.Customer.Name
	}

	shippingAddress := ""
	if addr, ok := shippingRecord["shipping_address"].(string); ok && addr != "" {
		shippingAddress = addr
	} else {
		shippingAddress = order.ContactAddress
		if shippingAddress == "" && order.Customer != nil {
			shippingAddress = order.Customer.Address
		}
	}

	contactPhone := order.ContactPhone
	if contactPhone == "" && order.Customer != nil {
		contactPhone = order.Customer.Phone
	}

	shippingDate := ""
	if date, ok := shippingRecord["shipping_date"].(string); ok {
		shippingDate = date
	}

	shippingStatus := ""
	if status, ok := shippingRecord["shipping_status"].(string); ok {
		statusMap := map[string]string{
			"pending":    "待出貨",
			"processing": "處理中",
			"shipped":    "已出貨",
			"delivered":  "已送達",
			"cancelled":  "已取消",
		}
		shippingStatus = statusMap[status]
	}

	shippingCompany := ""
	if company, ok := shippingRecord["shipping_company"].(string); ok {
		shippingCompany = company
	}

	trackingNumber := ""
	if tracking, ok := shippingRecord["tracking_number"].(string); ok {
		trackingNumber = tracking
	}

	estimatedDeliveryDate := ""
	if estDate, ok := shippingRecord["estimated_delivery_date"].(string); ok {
		estimatedDeliveryDate = estDate
	}

	actualDeliveryDate := ""
	if actDate, ok := shippingRecord["actual_delivery_date"].(string); ok {
		actualDeliveryDate = actDate
	}

	shippingFee := 0.0
	if fee, ok := shippingRecord["shipping_fee"].(float64); ok {
		shippingFee = fee
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>發貨單 - %s</title>
	<style>
		body { font-family: Arial, "Microsoft YaHei", sans-serif; margin: 20px; }
		.header { text-align: center; margin-bottom: 30px; }
		.company-info { margin-bottom: 20px; }
		.shipping-info { margin-bottom: 20px; }
		.customer-info { margin-bottom: 20px; }
		table { width: 100%%; border-collapse: collapse; margin-bottom: 20px; }
		th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
		th { background-color: #f2f2f2; }
		.total { text-align: right; font-weight: bold; font-size: 1.2em; }
		.terms { margin-top: 30px; padding-top: 20px; border-top: 2px solid #333; }
		.notes { margin-top: 20px; }
		@media print {
			body { margin: 0; }
		}
	</style>
</head>
<body>
	<div class="header">
		<h1>發貨單</h1>
		<h2>SHIPPING NOTE</h2>
	</div>
	
	<div class="company-info">
		%s
		<h3>%s</h3>
		%s
	</div>
	
	<div class="shipping-info">
		<p><strong>發貨單編號：</strong>%s</p>
		<p><strong>發貨日期：</strong>%s</p>
		<p><strong>訂單編號：</strong>%s</p>
		<p><strong>出貨狀態：</strong>%s</p>
		<p><strong>物流公司：</strong>%s</p>
		<p><strong>追蹤號碼：</strong>%s</p>
		<p><strong>預計送達日期：</strong>%s</p>
		<p><strong>實際送達日期：</strong>%s</p>
	</div>
	
	<div class="customer-info">
		<h3>收貨人信息</h3>
		<p><strong>收貨人：</strong>%s</p>
		<p><strong>地址：</strong>%s</p>
		<p><strong>電話：</strong>%s</p>
	</div>
	
	<table>
		<thead>
			<tr>
				<th>商品名稱</th>
				<th>數量</th>
				<th>單價</th>
				<th>小計</th>
			</tr>
		</thead>
		<tbody>`, shippingNumber, getEnterpriseLogoHTML(enterprise), getEnterpriseName(enterprise), getEnterpriseAddress(enterprise), shippingNumber, shippingDate, order.OrderNumber, shippingStatus, shippingCompany, trackingNumber, estimatedDeliveryDate, actualDeliveryDate, contactName, shippingAddress, contactPhone)

	for _, item := range order.OrderItems {
		productName := "商品"
		if item.Product != nil {
			productName = item.Product.Name
		}
		html += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%.2f</td>
				<td>$%.2f</td>
				<td>$%.2f</td>
			</tr>`, productName, item.Quantity, item.UnitPrice, item.TotalPrice)
	}

	html += fmt.Sprintf(`
		</tbody>
		<tfoot>
			<tr>
				<td colspan="3" class="total">運費：</td>
				<td>$%.2f</td>
			</tr>
		</tfoot>
	</table>
	
	<div class="terms">
		<h3>條款</h3>
		<div>%s</div>
	</div>
	
	<div class="notes">
		<h3>備註</h3>
		<div>%s</div>
	</div>
</body>
</html>`, shippingFee, settings.Terms, settings.Notes)

	return html
}

// GenerateServiceOrderContractPDF 為服務單生成合約 PDF
func GenerateServiceOrderContractPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	serviceOrderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid service order ID"})
	}

	// 獲取服務單
	var serviceOrder models.ServiceOrder
	if err := database.DB.Where("id = ? AND tenant_id = ?", serviceOrderID, tenantID).
		Preload("Customer").Preload("ServiceOrderItems.Service").Preload("ServiceOrderItems.Staff").Preload("Salesperson").
		First(&serviceOrder).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service order not found"})
	}

	// 獲取合約設定（如果有的話，否則使用發票設定）
	var contractSettings models.DocumentSettings
	if err := database.DB.Where("tenant_id = ? AND document_type = ?", tenantID, "contract").First(&contractSettings).Error; err != nil {
		// 如果沒有合約設定，使用發票設定
		if err := database.DB.Where("tenant_id = ? AND document_type = ?", tenantID, "invoice").First(&contractSettings).Error; err != nil {
			// 使用默認值
			contractSettings = models.DocumentSettings{
				Terms: "",
				Notes: "",
			}
		}
	}

	// 獲取企業信息
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	// 生成正式合約 PDF
	pdfBytes, err := buildServiceContractPDF(serviceOrder, contractSettings, enterprise)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate contract PDF: " + err.Error()})
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=service_contract_%s.pdf", serviceOrder.OrderNumber))
	return c.Send(pdfBytes)
}

// generateServiceOrderContractHTML 生成服務單合約 HTML
func generateServiceOrderContractHTML(serviceOrder models.ServiceOrder, settings models.DocumentSettings, enterprise models.Enterprise) string {
	customerName := "散客"
	if serviceOrder.CustomerID != nil {
		if serviceOrder.Customer.ID != uuid.Nil {
			customerName = serviceOrder.Customer.Name
			if serviceOrder.Customer.LastName != "" {
				// 格式化客戶名稱
				isChinese := false
				for _, r := range serviceOrder.Customer.LastName {
					if r >= 0x4e00 && r <= 0x9fff {
						isChinese = true
						break
					}
				}
				if isChinese {
					customerName = serviceOrder.Customer.LastName + customerName
				} else {
					customerName = customerName + " " + serviceOrder.Customer.LastName
				}
			}
		}
	}

	contactName := serviceOrder.ContactName
	if contactName == "" && serviceOrder.CustomerID != nil {
		if serviceOrder.Customer.ID != uuid.Nil {
			contactName = serviceOrder.Customer.Name
		}
	}

	contactAddress := serviceOrder.ContactAddress
	if contactAddress == "" && serviceOrder.CustomerID != nil {
		if serviceOrder.Customer.ID != uuid.Nil {
			contactAddress = serviceOrder.Customer.Address
		}
	}

	contactPhone := serviceOrder.ContactPhone
	if contactPhone == "" && serviceOrder.CustomerID != nil {
		if serviceOrder.Customer.ID != uuid.Nil {
			contactPhone = serviceOrder.Customer.Phone
		}
	}

	serviceDate := serviceOrder.ServiceDate.Format("2006-01-02")

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>服務合約 - %s</title>
	<style>
		body { font-family: Arial, "Microsoft YaHei", sans-serif; margin: 20px; }
		.header { text-align: center; margin-bottom: 30px; }
		.company-info { margin-bottom: 20px; }
		.contract-info { margin-bottom: 20px; }
		.customer-info { margin-bottom: 20px; }
		table { width: 100%%; border-collapse: collapse; margin-bottom: 20px; }
		th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
		th { background-color: #f2f2f2; }
		.total { text-align: right; font-weight: bold; font-size: 1.2em; }
		.terms { margin-top: 30px; padding-top: 20px; border-top: 2px solid #333; }
		.notes { margin-top: 20px; }
		.signature { margin-top: 50px; }
		.signature-row { display: flex; justify-content: space-between; margin-top: 50px; }
		.signature-box { width: 45%%; text-align: center; }
		@media print {
			@page {
				margin: 1cm;
			}
			body { margin: 0; }
		}
	</style>
</head>
<body>
	<div class="header">
		<h1>服務合約</h1>
		<h2>SERVICE CONTRACT</h2>
	</div>
	
	<div class="company-info">
		%s
		<h3>%s</h3>
		%s
	</div>
	
	<div class="contract-info">
		<p><strong>合約編號：</strong>%s</p>
		<p><strong>服務日期：</strong>%s</p>
		<p><strong>服務單號：</strong>%s</p>
	</div>
	
	<div class="customer-info">
		<h3>客戶信息</h3>
		<p><strong>客戶名稱：</strong>%s</p>
		<p><strong>聯絡人：</strong>%s</p>
		<p><strong>電話：</strong>%s</p>
		<p><strong>地址：</strong>%s</p>
	</div>
	
	<h3>服務明細</h3>
	<table>
		<thead>
			<tr>
				<th>服務項目</th>
				<th>服務員</th>
				<th>數量</th>
				<th>單價</th>
				<th>小計</th>
			</tr>
		</thead>
		<tbody>`,
		serviceOrder.OrderNumber,
		getEnterpriseLogoHTML(enterprise),
		getEnterpriseName(enterprise),
		getEnterpriseAddress(enterprise),
		serviceOrder.OrderNumber,
		serviceDate,
		serviceOrder.OrderNumber,
		customerName,
		contactName,
		contactPhone,
		contactAddress)

	// 添加服務明細
	for _, item := range serviceOrder.ServiceOrderItems {
		serviceName := "自定義服務"
		if item.Service != nil {
			serviceName = item.Service.Name
		}

		staffName := "-"
		if item.Staff != nil {
			staffName = item.Staff.Name
		}

		html += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%.2f</td>
				<td>$%.2f</td>
				<td>$%.2f</td>
			</tr>`, serviceName, staffName, item.Quantity, item.UnitPrice, item.TotalPrice)
	}

	html += fmt.Sprintf(`
		</tbody>
		<tfoot>
			<tr>
				<td colspan="4" class="total">總計：</td>
				<td>$%.2f</td>
			</tr>
		</tfoot>
	</table>
	
	<div class="terms">
		<h3>條款</h3>
		<div>%s</div>
	</div>
	
	<div class="notes">
		<h3>備註</h3>
		<div>%s</div>
	</div>
	
	<div class="signature">
		<div class="signature-row">
			<div class="signature-box">
				<p>客戶簽名</p>
				<p style="border-top: 1px solid #333; margin-top: 50px; padding-top: 10px;">%s</p>
			</div>
			<div class="signature-box">
				<p>服務提供方簽名</p>
				<p style="border-top: 1px solid #333; margin-top: 50px; padding-top: 10px;">%s</p>
			</div>
		</div>
	</div>
</body>
</html>`,
		serviceOrder.TotalAmount,
		settings.Terms,
		serviceOrder.Notes,
		contactName,
		getEnterpriseName(enterprise))

	return html
}

// GenerateReceivingNotePDF 為收貨記錄生成收貨單 PDF
func GenerateReceivingNotePDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	purchaseOrderID, err := uuid.Parse(c.Params("purchaseOrderId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid purchase order ID"})
	}

	recordIndex, err := strconv.Atoi(c.Params("index"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid record index"})
	}

	// 獲取採購單
	var po models.PurchaseOrder
	if err := database.DB.Where("id = ? AND tenant_id = ?", purchaseOrderID, tenantID).
		Preload("Supplier").Preload("PurchaseOrderItems.Product").First(&po).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order not found"})
	}

	// 從 ExtraFields 中提取 receiving_notes
	var receivingNotes []map[string]interface{}
	if po.ExtraFields != nil {
		fields := map[string]interface{}(po.ExtraFields)
		if notes, exists := fields["receiving_notes"]; exists {
			if notesList, ok := notes.([]interface{}); ok {
				for _, n := range notesList {
					if note, ok := n.(map[string]interface{}); ok {
						receivingNotes = append(receivingNotes, note)
					}
				}
			}
		}
	}

	if recordIndex < 0 || recordIndex >= len(receivingNotes) {
		return c.Status(404).JSON(fiber.Map{"error": "Receiving note not found"})
	}

	receivingNote := receivingNotes[recordIndex]

	// 獲取收貨單編號
	receivingNumber := ""
	if receivingNum, ok := receivingNote["receiving_number"].(string); ok && receivingNum != "" {
		receivingNumber = receivingNum
	} else {
		today := time.Now().Format("20060102")
		datePrefix := "RECV-" + today + "-"
		var count int64
		var purchaseOrders []models.PurchaseOrder
		database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").Find(&purchaseOrders)
		count = int64(len(purchaseOrders))
		receivingNumber = datePrefix + fmt.Sprintf("%03d", count+1)

		receivingNote["receiving_number"] = receivingNumber
		if po.ExtraFields != nil {
			fields := map[string]interface{}(po.ExtraFields)
			fields["receiving_notes"] = receivingNotes
			po.ExtraFields = models.JSONB(fields)
			database.DB.Save(&po)
		}
	}

	// 獲取企業信息
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	// 生成收貨單 PDF
	headers := []string{"商品名稱", "數量"}
	rows := make([][]string, 0)

	if items, exists := receivingNote["items"].([]interface{}); exists && len(items) > 0 {
		// 使用 receivingNote 中的 items
		for _, itemInterface := range items {
			if item, ok := itemInterface.(map[string]interface{}); ok {
				productID, _ := item["product_id"].(string)
				quantity := 0.0
				if qty, ok := item["quantity"].(float64); ok {
					quantity = qty
				}

				// 查找產品名稱
				productName := "商品"
				if productID != "" {
					var product models.Product
					if err := database.DB.Where("id = ? AND tenant_id = ?", productID, tenantID).First(&product).Error; err == nil {
						productName = product.Name
					}
				}

				rows = append(rows, []string{productName, fmt.Sprintf("%.2f", quantity)})
			}
		}
	} else {
		// 向後兼容：使用整個採購單的 PurchaseOrderItems
		for _, item := range po.PurchaseOrderItems {
			name := "商品"
			if item.Product != nil {
				name = item.Product.Name
			}
			rows = append(rows, []string{name, fmt.Sprintf("%.2f", float64(item.Quantity))})
		}
	}

	pdfBytes, err := buildReceivingNotePDF(po, receivingNote, receivingNumber, enterprise, tenantID, headers, rows)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate receiving note PDF: " + err.Error()})
	}
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=receiving_note_%s.pdf", receivingNumber))
	return c.Send(pdfBytes)
}

// GenerateOrderRefundNotePDF 為訂單退款單生成 PDF
func GenerateOrderRefundNotePDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	orderID, err := uuid.Parse(c.Params("orderId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order ID"})
	}
	noteIndex, err := strconv.Atoi(c.Params("index"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid note index"})
	}

	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", orderID, tenantID).
		Preload("Customer").Preload("OrderItems.Product").First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}

	// 取得 refund_notes
	var refundNotes []map[string]interface{}
	if order.ExtraFields != nil {
		fields := map[string]interface{}(order.ExtraFields)
		if notesAny, exists := fields["refund_notes"]; exists {
			if notesList, ok := notesAny.([]interface{}); ok {
				for _, n := range notesList {
					if m, ok := n.(map[string]interface{}); ok {
						refundNotes = append(refundNotes, m)
					}
				}
			}
		}
	}
	if noteIndex < 0 || noteIndex >= len(refundNotes) {
		return c.Status(404).JSON(fiber.Map{"error": "Refund note not found"})
	}
	note := refundNotes[noteIndex]
	refundNumber, _ := note["refund_number"].(string)
	if refundNumber == "" {
		refundNumber = fmt.Sprintf("#%d", noteIndex+1)
	}

	// product name map
	productNameByID := map[string]string{}
	for _, oi := range order.OrderItems {
		if oi.ProductID != nil && oi.Product != nil {
			productNameByID[oi.ProductID.String()] = oi.Product.Name
		}
	}

	// 獲取企業信息
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	customerName := "散客"
	if order.Customer != nil && order.Customer.Name != "" {
		customerName = order.Customer.Name
	}

	headers := []string{"產品", "數量", "單價", "小計"}
	rows := make([][]string, 0)
	itemsTotal := 0.0

	if itemsAny, ok := note["items"].([]interface{}); ok {
		for _, ir := range itemsAny {
			im, ok := ir.(map[string]interface{})
			if !ok {
				continue
			}
			pid, _ := im["product_id"].(string)
			name := productNameByID[pid]
			if name == "" {
				name = pid
			}
			qty := 0.0
			if v, ok := im["quantity"].(float64); ok {
				qty = v
			}
			unit := 0.0
			if v, ok := im["unit_price"].(float64); ok {
				unit = v
			}
			sub := qty * unit
			itemsTotal += sub
			rows = append(rows, []string{
				name,
				fmt.Sprintf("%.2f", qty),
				fmt.Sprintf("%.2f", unit),
				fmt.Sprintf("%.2f", sub),
			})
		}
	}

	extraAmount := 0.0
	if v, ok := note["extra_amount"].(float64); ok {
		extraAmount = v
	}
	if extraAmount != 0 {
		rows = append(rows, []string{"額外款項", "", "", fmt.Sprintf("%.2f", extraAmount)})
	}
	total := itemsTotal + extraAmount
	rows = append(rows, []string{"合計", "", "", fmt.Sprintf("%.2f", total)})

	title := fmt.Sprintf("退款單 %s（訂單 %s / %s）", refundNumber, order.OrderNumber, customerName)
	pdfBytes, err := buildRefundNotePDF(title, enterprise, tenantID, headers, rows)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate refund note PDF: " + err.Error()})
	}
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=refund_note_%s.pdf", refundNumber))
	return c.Send(pdfBytes)
}

// GenerateServiceOrderRefundNotePDF 為服務單退款單生成 PDF
func GenerateServiceOrderRefundNotePDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	serviceOrderID, err := uuid.Parse(c.Params("serviceOrderId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid service order ID"})
	}
	noteIndex, err := strconv.Atoi(c.Params("index"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid note index"})
	}

	var so models.ServiceOrder
	if err := database.DB.Where("id = ? AND tenant_id = ?", serviceOrderID, tenantID).
		Preload("Customer").Preload("ServiceOrderItems.Service").First(&so).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service order not found"})
	}

	var refundNotes []map[string]interface{}
	if so.ExtraFields != nil {
		fields := map[string]interface{}(so.ExtraFields)
		if notesAny, exists := fields["refund_notes"]; exists {
			if notesList, ok := notesAny.([]interface{}); ok {
				for _, n := range notesList {
					if m, ok := n.(map[string]interface{}); ok {
						refundNotes = append(refundNotes, m)
					}
				}
			}
		}
	}
	if noteIndex < 0 || noteIndex >= len(refundNotes) {
		return c.Status(404).JSON(fiber.Map{"error": "Refund note not found"})
	}
	note := refundNotes[noteIndex]
	refundNumber, _ := note["refund_number"].(string)
	if refundNumber == "" {
		refundNumber = fmt.Sprintf("#%d", noteIndex+1)
	}

	serviceNameByID := map[string]string{}
	for _, it := range so.ServiceOrderItems {
		if it.ServiceID != nil && it.Service != nil {
			serviceNameByID[it.ServiceID.String()] = it.Service.Name
		}
	}

	// 獲取企業信息
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	customerName := "散客"
	if so.Customer.ID != uuid.Nil && so.Customer.Name != "" {
		customerName = so.Customer.Name
	}

	headers := []string{"服務", "數量", "單價", "小計"}
	rows := make([][]string, 0)
	itemsTotal := 0.0

	if itemsAny, ok := note["items"].([]interface{}); ok {
		for _, ir := range itemsAny {
			im, ok := ir.(map[string]interface{})
			if !ok {
				continue
			}
			sid, _ := im["service_id"].(string)
			name := serviceNameByID[sid]
			if name == "" {
				name = sid
			}
			qty := 0.0
			if v, ok := im["quantity"].(float64); ok {
				qty = v
			}
			unit := 0.0
			if v, ok := im["unit_price"].(float64); ok {
				unit = v
			}
			sub := qty * unit
			itemsTotal += sub
			rows = append(rows, []string{name, fmt.Sprintf("%.2f", qty), fmt.Sprintf("%.2f", unit), fmt.Sprintf("%.2f", sub)})
		}
	}

	extraAmount := 0.0
	if v, ok := note["extra_amount"].(float64); ok {
		extraAmount = v
	}
	if extraAmount != 0 {
		rows = append(rows, []string{"額外款項", "", "", fmt.Sprintf("%.2f", extraAmount)})
	}
	total := itemsTotal + extraAmount
	rows = append(rows, []string{"合計", "", "", fmt.Sprintf("%.2f", total)})

	title := fmt.Sprintf("退款單 %s（服務單 %s / %s）", refundNumber, so.OrderNumber, customerName)
	pdfBytes, err := buildRefundNotePDF(title, enterprise, tenantID, headers, rows)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate refund note PDF: " + err.Error()})
	}
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=refund_note_%s.pdf", refundNumber))
	return c.Send(pdfBytes)
}

// setupCJKFont 安全地設置中文字體，如果失敗則回退到 Helvetica
func setupCJKFont(pdf *gofpdf.Fpdf) (fontName string, hasBold bool) {
	fontPath := utils.FindCJKFontPath()
	fontName = "NotoCJK"
	hasBold = false

	if fontPath == "" {
		// 沒找到任何 CJK 字體，直接使用 Helvetica
		fontName = "Helvetica"
		pdf.SetFont(fontName, "", 10)
		hasBold = true
		return fontName, hasBold
	}

	// 清理並正規化路徑
	fontPath = strings.TrimSpace(fontPath)

	// 修正 Linux 上缺少前導斜線的路徑
	if runtime.GOOS != "windows" && !strings.HasPrefix(fontPath, "/") {
		if strings.HasPrefix(fontPath, "usr/") {
			fontPath = "/" + fontPath
		} else {
			// 非標準路徑，跳過
			fontName = "Helvetica"
			pdf.SetFont(fontName, "", 10)
			hasBold = true
			return fontName, hasBold
		}
	}

	// 確保路徑是絕對路徑且文件存在
	var absPath string
	var err error

	// 如果已經是絕對路徑（Linux 以 / 開頭，Windows 以盤符開頭），直接使用
	if filepath.IsAbs(fontPath) {
		absPath = fontPath
	} else {
		// 轉換為絕對路徑
		absPath, err = filepath.Abs(fontPath)
		if err != nil {
			fontName = "Helvetica"
			pdf.SetFont(fontName, "", 10)
			hasBold = true
			return fontName, hasBold
		}
	}

	// 清理路徑（去除 .. 和 .）
	absPath = filepath.Clean(absPath)

	// 對於 Linux，確保路徑以 / 開頭
	if runtime.GOOS != "windows" {
		// 確保使用正斜杠（gofpdf 可能對路徑格式敏感）
		absPath = filepath.ToSlash(absPath)
		if !strings.HasPrefix(absPath, "/") {
			absPath = "/" + absPath
		}
	}

	// 再次驗證文件是否存在且可讀
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		fontName = "Helvetica"
		pdf.SetFont(fontName, "", 10)
		hasBold = true
		return fontName, hasBold
	}

	// 檢查文件擴展名
	ext := strings.ToLower(filepath.Ext(absPath))
	if ext != ".ttf" && ext != ".ttc" && ext != ".otf" {
		fontName = "Helvetica"
		pdf.SetFont(fontName, "", 10)
		hasBold = true
		return fontName, hasBold
	}

	// 嘗試添加字體
	// 最終防線：確保 Linux 絕對路徑以 / 開頭
	if runtime.GOOS != "windows" && !strings.HasPrefix(absPath, "/") {
		absPath = "/" + absPath
	}
	log.Printf("[PDF] setupCJKFont: using font path: %s", absPath)

	// 直接讀取字體檔案內容，避免 gofpdf 內部路徑處理問題
	fontBytes, err := os.ReadFile(absPath)
	if err != nil {
		log.Printf("[PDF] Failed to read font file: %v", err)
		fontName = "Helvetica"
		pdf.SetFont(fontName, "", 10)
		hasBold = true
		return fontName, hasBold
	}
	pdf.AddUTF8FontFromBytes(fontName, "", fontBytes)
	pdf.SetFont(fontName, "", 10)
	return fontName, hasBold
}

// buildInvoicePDF 使用 gofpdf 構建完整的發票 PDF
// isQuotation: true 表示報價單，false 表示發票
func buildInvoicePDF(order models.Order, paymentRecord map[string]interface{}, invoiceNumber string, settings models.DocumentSettings, enterprise models.Enterprise, isQuotation bool) ([]byte, error) {
	// 創建 PDF（直向）
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(36, 36, 36)
	pdf.SetAutoPageBreak(true, 36)

	// 設置中文字體
	fontName, hasBold := setupCJKFont(pdf)

	// 讀取全域文件樣式設定
	styleSettings := GetDocumentStyleSettings(settings.TenantID)
	titleSize := models.DefaultTitleFontSize
	subtitleSize := models.DefaultSubtitleFontSize
	bodySize := models.DefaultBodyFontSize
	sectionSize := models.DefaultSectionFontSize
	notesSize := models.DefaultNotesFontSize
	if styleSettings != nil {
		titleSize = styleSettings.GetTitleFontSize()
		bodySize = styleSettings.GetBodyFontSize()
		notesSize = styleSettings.GetNotesFontSize()
		subtitleSize = titleSize * 0.72
		sectionSize = bodySize + 2
	}

	pdf.AddPage()
	margin := 36.0
	pageW, pageH := pdf.GetPageSize()
	usableW := pageW - margin*2
	lineH := bodySize + 3
	currentY := margin

	// 標題（粗體）
	if hasBold {
		pdf.SetFont(fontName, "B", titleSize)
	} else {
		pdf.SetFont(fontName, "", titleSize)
	}
	pdf.SetXY(margin, currentY)
	if isQuotation {
		pdf.CellFormat(usableW, lineH*1.5, "報價單", "", 0, "C", false, 0, "")
		pdf.SetFont(fontName, "", subtitleSize)
		pdf.SetXY(margin, currentY+lineH*1.5)
		pdf.CellFormat(usableW, lineH, "QUOTATION", "", 0, "C", false, 0, "")
	} else {
		pdf.CellFormat(usableW, lineH*1.5, "訂單發票", "", 0, "C", false, 0, "")
		pdf.SetFont(fontName, "", subtitleSize)
		pdf.SetXY(margin, currentY+lineH*1.5)
		pdf.CellFormat(usableW, lineH, "ORDER INVOICE", "", 0, "C", false, 0, "")
	}
	currentY += lineH * 3

	// 企業信息（使用文件 Logo）
	logoHeight, logoCleanup := drawDocumentLogo(pdf, enterprise, styleSettings, fontName, hasBold, margin, currentY, usableW, lineH)
	if logoCleanup != nil {
		defer logoCleanup()
	}
	currentY += logoHeight

	pdf.SetFont(fontName, "", bodySize)
	enterpriseAddr := getEnterpriseAddress(enterprise)
	if enterpriseAddr != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, enterpriseAddr, "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// 發票/報價單信息
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetXY(margin, currentY)
	if isQuotation {
		pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("報價單號：%s", invoiceNumber), "", 0, "L", false, 0, "")
	} else {
		pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("發票編號：%s", invoiceNumber), "", 0, "L", false, 0, "")
	}

	paymentDate := ""
	if date, ok := paymentRecord["payment_date"].(string); ok {
		paymentDate = date
	}
	pdf.SetXY(margin+usableW/2, currentY)
	if isQuotation {
		pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("報價日期：%s", paymentDate), "", 1, "L", false, 0, "")
	} else {
		pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("發票日期：%s", paymentDate), "", 1, "L", false, 0, "")
	}
	currentY += lineH

	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, fmt.Sprintf("訂單編號：%s", order.OrderNumber), "", 1, "L", false, 0, "")
	currentY += lineH * 1.5

	// 客戶信息
	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, "客戶信息", "", 1, "L", false, 0, "")
	currentY += lineH

	pdf.SetFont(fontName, "", bodySize)
	customerName := "散客"
	if order.Customer != nil {
		customerName = order.Customer.Name
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, fmt.Sprintf("客戶名稱：%s", customerName), "", 1, "L", false, 0, "")
	currentY += lineH

	contactName := order.ContactName
	if contactName == "" && order.Customer != nil {
		contactName = order.Customer.Name
	}
	if contactName != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("聯絡人：%s", contactName), "", 1, "L", false, 0, "")
		currentY += lineH
	}

	contactAddress := order.ContactAddress
	if contactAddress == "" && order.Customer != nil {
		contactAddress = order.Customer.Address
	}
	if contactAddress != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("地址：%s", contactAddress), "", 1, "L", false, 0, "")
		currentY += lineH
	}

	contactPhone := order.ContactPhone
	if contactPhone == "" && order.Customer != nil {
		contactPhone = order.Customer.Phone
	}
	if contactPhone != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("電話：%s", contactPhone), "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// 商品明細表格
	colW := []float64{usableW * 0.4, usableW * 0.15, usableW * 0.2, usableW * 0.25}

	// 表頭
	if hasBold {
		pdf.SetFont(fontName, "B", bodySize)
	} else {
		pdf.SetFont(fontName, "", bodySize)
	}
	pdf.SetFillColor(240, 240, 240)
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0], lineH*1.2, "商品名稱", "1", 0, "L", true, 0, "")
	pdf.CellFormat(colW[1], lineH*1.2, "數量", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colW[2], lineH*1.2, "單價", "1", 0, "R", true, 0, "")
	pdf.CellFormat(colW[3], lineH*1.2, "小計", "1", 1, "R", true, 0, "")
	currentY += lineH * 1.2

	// 商品行
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetFillColor(255, 255, 255)
	totalAmount := 0.0
	for _, item := range order.OrderItems {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		productName := "商品"
		if item.Product != nil {
			productName = item.Product.Name
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(colW[0], lineH, productName, "1", 0, "L", false, 0, "")
		pdf.CellFormat(colW[1], lineH, fmt.Sprintf("%.2f", item.Quantity), "1", 0, "C", false, 0, "")
		pdf.CellFormat(colW[2], lineH, fmt.Sprintf("$%.2f", item.UnitPrice), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[3], lineH, fmt.Sprintf("$%.2f", item.TotalPrice), "1", 1, "R", false, 0, "")
		currentY += lineH
		totalAmount += item.TotalPrice
	}

	// 付款信息
	if currentY > pageH-margin-lineH*5 {
		pdf.AddPage()
		currentY = margin
	}
	pdf.SetFont(fontName, "", bodySize)
	paymentMethod := ""
	if method, ok := paymentRecord["payment_method"].(string); ok {
		methodMap := map[string]string{
			"cash":          "現金",
			"credit_card":   "信用卡",
			"bank_transfer": "銀行轉帳",
			"check":         "支票",
			"other":         "其他",
		}
		paymentMethod = methodMap[method]
		if paymentMethod == "" {
			paymentMethod = method
		}
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH, "付款方式：", "1", 0, "R", false, 0, "")
	pdf.CellFormat(colW[3], lineH, paymentMethod, "1", 1, "L", false, 0, "")
	currentY += lineH

	referenceNumber := ""
	if ref, ok := paymentRecord["reference_number"].(string); ok {
		referenceNumber = ref
	}
	if referenceNumber != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH, "參考號碼：", "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[3], lineH, referenceNumber, "1", 1, "L", false, 0, "")
		currentY += lineH
	}

	amount := 0.0
	if amt, ok := paymentRecord["amount"].(float64); ok {
		amount = amt
	} else {
		amount = totalAmount
	}
	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH*1.2, "應付總額：", "1", 0, "R", false, 0, "")
	pdf.CellFormat(colW[3], lineH*1.2, fmt.Sprintf("$%.2f", amount), "1", 1, "R", false, 0, "")
	currentY += lineH * 1.5

	// 發票條款和發票備註（僅限發票，報價單不顯示）
	if !isQuotation {
		if settings.Terms != "" {
			if currentY > pageH-margin-lineH*8 {
				pdf.AddPage()
				currentY = margin
			}
			if hasBold {
				pdf.SetFont(fontName, "B", bodySize)
			} else {
				pdf.SetFont(fontName, "", bodySize)
			}
			pdf.SetXY(margin, currentY)
			pdf.CellFormat(usableW, lineH, "發票條款", "", 1, "L", false, 0, "")
			currentY += lineH
			pdf.SetFont(fontName, "", notesSize)
			pdf.SetXY(margin, currentY)
			pdf.MultiCell(usableW, lineH*0.8, settings.Terms, "", "L", false)
			currentY = pdf.GetY() + lineH*0.5
		}

		// 合併備註：document-settings 備註 + 付款備註
		mergedNotes := strings.TrimSpace(settings.Notes)
		paymentNotes := ""
		if pn, ok := paymentRecord["notes"].(string); ok {
			paymentNotes = strings.TrimSpace(pn)
		}
		if mergedNotes != "" && paymentNotes != "" {
			mergedNotes = mergedNotes + "\n" + paymentNotes
		} else if paymentNotes != "" {
			mergedNotes = paymentNotes
		}

		if mergedNotes != "" {
			if currentY > pageH-margin-lineH*5 {
				pdf.AddPage()
				currentY = margin
			}
			if hasBold {
				pdf.SetFont(fontName, "B", bodySize)
			} else {
				pdf.SetFont(fontName, "", bodySize)
			}
			pdf.SetXY(margin, currentY)
			pdf.CellFormat(usableW, lineH, "發票備註", "", 1, "L", false, 0, "")
			currentY += lineH
			pdf.SetFont(fontName, "", notesSize)
			pdf.SetXY(margin, currentY)
			pdf.MultiCell(usableW, lineH*0.8, mergedNotes, "", "L", false)
			currentY = pdf.GetY() + lineH*0.5
		}
	}

	// 訂單備註（發票和報價單都顯示）
	orderNotes := strings.TrimSpace(order.Notes)
	if orderNotes != "" {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "訂單備註", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.SetXY(margin, currentY)
		pdf.MultiCell(usableW, lineH*0.8, orderNotes, "", "L", false)
	}

	// 輸出 PDF
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildServiceInvoicePDF 使用 gofpdf 構建服務單付款發票 PDF（樣式同訂單）
func buildServiceInvoicePDF(so models.ServiceOrder, paymentRecord map[string]interface{}, invoiceNumber string, settings models.DocumentSettings, enterprise models.Enterprise) ([]byte, error) {
	// 創建 PDF（直向）
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(36, 36, 36)
	pdf.SetAutoPageBreak(true, 36)

	// 設置中文字體
	fontName, hasBold := setupCJKFont(pdf)

	// 讀取全域文件樣式設定
	styleSettings := GetDocumentStyleSettings(settings.TenantID)
	titleSize := models.DefaultTitleFontSize
	subtitleSize := models.DefaultSubtitleFontSize
	bodySize := models.DefaultBodyFontSize
	sectionSize := models.DefaultSectionFontSize
	notesSize := models.DefaultNotesFontSize
	if styleSettings != nil {
		titleSize = styleSettings.GetTitleFontSize()
		bodySize = styleSettings.GetBodyFontSize()
		notesSize = styleSettings.GetNotesFontSize()
		subtitleSize = titleSize * 0.72
		sectionSize = bodySize + 2
	}

	pdf.AddPage()
	margin := 36.0
	pageW, pageH := pdf.GetPageSize()
	usableW := pageW - margin*2
	lineH := bodySize + 3
	currentY := margin

	// 標題（粗體）
	if hasBold {
		pdf.SetFont(fontName, "B", titleSize)
	} else {
		pdf.SetFont(fontName, "", titleSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH*1.5, "服務單發票", "", 0, "C", false, 0, "")
	pdf.SetFont(fontName, "", subtitleSize)
	pdf.SetXY(margin, currentY+lineH*1.5)
	pdf.CellFormat(usableW, lineH, "SERVICE ORDER INVOICE", "", 0, "C", false, 0, "")
	currentY += lineH * 3

	// 企業信息（使用文件 Logo）
	logoHeight, logoCleanup := drawDocumentLogo(pdf, enterprise, styleSettings, fontName, hasBold, margin, currentY, usableW, lineH)
	if logoCleanup != nil {
		defer logoCleanup()
	}
	currentY += logoHeight

	pdf.SetFont(fontName, "", bodySize)
	enterpriseAddr := getEnterpriseAddress(enterprise)
	if enterpriseAddr != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, enterpriseAddr, "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// 發票信息
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("發票編號：%s", invoiceNumber), "", 0, "L", false, 0, "")

	paymentDate := ""
	if date, ok := paymentRecord["payment_date"].(string); ok {
		paymentDate = date
	}
	pdf.SetXY(margin+usableW/2, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("發票日期：%s", paymentDate), "", 1, "L", false, 0, "")
	currentY += lineH

	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, fmt.Sprintf("服務單號：%s", so.OrderNumber), "", 1, "L", false, 0, "")
	currentY += lineH * 1.5

	// 客戶信息
	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, "客戶信息", "", 1, "L", false, 0, "")
	currentY += lineH

	pdf.SetFont(fontName, "", bodySize)
	customerName := "散客"
	if so.Customer.ID != uuid.Nil {
		customerName = so.Customer.Name
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, fmt.Sprintf("客戶名稱：%s", customerName), "", 1, "L", false, 0, "")
	currentY += lineH

	contactName := so.ContactName
	if contactName == "" && so.Customer.ID != uuid.Nil {
		contactName = so.Customer.Name
	}
	if contactName != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("聯絡人：%s", contactName), "", 1, "L", false, 0, "")
		currentY += lineH
	}

	contactAddress := so.ContactAddress
	if contactAddress == "" && so.Customer.ID != uuid.Nil {
		contactAddress = so.Customer.Address
	}
	if contactAddress != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("地址：%s", contactAddress), "", 1, "L", false, 0, "")
		currentY += lineH
	}

	contactPhone := so.ContactPhone
	if contactPhone == "" && so.Customer.ID != uuid.Nil {
		contactPhone = so.Customer.Phone
	}
	if contactPhone != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("電話：%s", contactPhone), "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// 服務明細表格
	colW := []float64{usableW * 0.4, usableW * 0.15, usableW * 0.2, usableW * 0.25}

	// 表頭
	if hasBold {
		pdf.SetFont(fontName, "B", bodySize)
	} else {
		pdf.SetFont(fontName, "", bodySize)
	}
	pdf.SetFillColor(240, 240, 240)
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0], lineH*1.2, "服務名稱", "1", 0, "L", true, 0, "")
	pdf.CellFormat(colW[1], lineH*1.2, "數量", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colW[2], lineH*1.2, "單價", "1", 0, "R", true, 0, "")
	pdf.CellFormat(colW[3], lineH*1.2, "小計", "1", 1, "R", true, 0, "")
	currentY += lineH * 1.2

	// 服務行
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetFillColor(255, 255, 255)
	totalAmount := 0.0
	for _, item := range so.ServiceOrderItems {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		serviceName := "服務"
		if item.Service != nil {
			serviceName = item.Service.Name
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(colW[0], lineH, serviceName, "1", 0, "L", false, 0, "")
		pdf.CellFormat(colW[1], lineH, fmt.Sprintf("%.2f", item.Quantity), "1", 0, "C", false, 0, "")
		pdf.CellFormat(colW[2], lineH, fmt.Sprintf("$%.2f", item.UnitPrice), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[3], lineH, fmt.Sprintf("$%.2f", item.TotalPrice), "1", 1, "R", false, 0, "")
		currentY += lineH
		totalAmount += item.TotalPrice
	}

	// 付款信息
	if currentY > pageH-margin-lineH*5 {
		pdf.AddPage()
		currentY = margin
	}
	pdf.SetFont(fontName, "", bodySize)
	paymentMethod := ""
	if method, ok := paymentRecord["payment_method"].(string); ok {
		methodMap := map[string]string{
			"cash":          "現金",
			"credit_card":   "信用卡",
			"bank_transfer": "銀行轉帳",
			"check":         "支票",
			"other":         "其他",
		}
		paymentMethod = methodMap[method]
		if paymentMethod == "" {
			paymentMethod = method
		}
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH, "付款方式：", "1", 0, "R", false, 0, "")
	pdf.CellFormat(colW[3], lineH, paymentMethod, "1", 1, "L", false, 0, "")
	currentY += lineH

	referenceNumber := ""
	if ref, ok := paymentRecord["reference_number"].(string); ok {
		referenceNumber = ref
	}
	if referenceNumber != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH, "參考號碼：", "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[3], lineH, referenceNumber, "1", 1, "L", false, 0, "")
		currentY += lineH
	}

	amount := 0.0
	if amt, ok := paymentRecord["amount"].(float64); ok {
		amount = amt
	} else {
		amount = totalAmount
	}
	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH*1.2, "應付總額：", "1", 0, "R", false, 0, "")
	pdf.CellFormat(colW[3], lineH*1.2, fmt.Sprintf("$%.2f", amount), "1", 1, "R", false, 0, "")
	currentY += lineH * 1.5

	// 條款和備註
	if settings.Terms != "" {
		if currentY > pageH-margin-lineH*8 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "條款", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.SetXY(margin, currentY)
		pdf.MultiCell(usableW, lineH*0.8, settings.Terms, "", "L", false)
		currentY = pdf.GetY() + lineH*0.5
	}

	if settings.Notes != "" {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "備註", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.SetXY(margin, currentY)
		pdf.MultiCell(usableW, lineH*0.8, settings.Notes, "", "L", false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildPurchasePaymentPDF 生成採購支出單（採購付款記錄）
func buildPurchasePaymentPDF(po models.PurchaseOrder, paymentRecord map[string]interface{}, invoiceNumber string, settings models.DocumentSettings, enterprise models.Enterprise) ([]byte, error) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(36, 36, 36)
	pdf.SetAutoPageBreak(true, 36)

	// 設置中文字體
	fontName, hasBold := setupCJKFont(pdf)

	// 讀取全域文件樣式設定
	styleSettings := GetDocumentStyleSettings(settings.TenantID)
	titleSize := models.DefaultTitleFontSize
	subtitleSize := models.DefaultSubtitleFontSize
	bodySize := models.DefaultBodyFontSize
	sectionSize := models.DefaultSectionFontSize
	notesSize := models.DefaultNotesFontSize
	if styleSettings != nil {
		titleSize = styleSettings.GetTitleFontSize()
		bodySize = styleSettings.GetBodyFontSize()
		notesSize = styleSettings.GetNotesFontSize()
		subtitleSize = titleSize * 0.72
		sectionSize = bodySize + 2
	}

	pdf.AddPage()
	margin := 36.0
	pageW, pageH := pdf.GetPageSize()
	usableW := pageW - margin*2
	lineH := bodySize + 3
	currentY := margin

	// 標題（粗體）
	if hasBold {
		pdf.SetFont(fontName, "B", titleSize)
	} else {
		pdf.SetFont(fontName, "", titleSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH*1.5, "採購支出單", "", 0, "C", false, 0, "")
	pdf.SetFont(fontName, "", subtitleSize)
	pdf.SetXY(margin, currentY+lineH*1.5)
	pdf.CellFormat(usableW, lineH, "PURCHASE PAYMENT", "", 0, "C", false, 0, "")
	currentY += lineH * 3

	// 企業信息（使用文件 Logo）
	logoHeight, logoCleanup := drawDocumentLogo(pdf, enterprise, styleSettings, fontName, hasBold, margin, currentY, usableW, lineH)
	if logoCleanup != nil {
		defer logoCleanup()
	}
	currentY += logoHeight
	pdf.SetFont(fontName, "", bodySize)
	if addr := getEnterpriseAddress(enterprise); addr != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, addr, "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// 單據信息
	pdf.SetFont(fontName, "", bodySize)
	paymentDate := ""
	if date, ok := paymentRecord["payment_date"].(string); ok {
		paymentDate = date
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("支出單號：%s", invoiceNumber), "", 0, "L", false, 0, "")
	pdf.SetXY(margin+usableW/2, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("支出日期：%s", paymentDate), "", 1, "L", false, 0, "")
	currentY += lineH
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, fmt.Sprintf("採購單號：%s", po.OrderNumber), "", 1, "L", false, 0, "")
	currentY += lineH * 1.5

	// 供應商信息
	supplierName := "供應商"
	if po.Supplier != nil {
		supplierName = po.Supplier.Name
	}
	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, "供應商", "", 1, "L", false, 0, "")
	currentY += lineH
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, supplierName, "", 1, "L", false, 0, "")
	currentY += lineH * 1.5

	// 採購明細
	colW := []float64{usableW * 0.4, usableW * 0.15, usableW * 0.2, usableW * 0.25}
	if hasBold {
		pdf.SetFont(fontName, "B", bodySize)
	} else {
		pdf.SetFont(fontName, "", bodySize)
	}
	pdf.SetFillColor(240, 240, 240)
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0], lineH*1.2, "品項", "1", 0, "L", true, 0, "")
	pdf.CellFormat(colW[1], lineH*1.2, "數量", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colW[2], lineH*1.2, "單價", "1", 0, "R", true, 0, "")
	pdf.CellFormat(colW[3], lineH*1.2, "小計", "1", 1, "R", true, 0, "")
	currentY += lineH * 1.2

	pdf.SetFont(fontName, "", bodySize)
	pdf.SetFillColor(255, 255, 255)
	totalAmount := 0.0
	for _, item := range po.PurchaseOrderItems {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		name := ""
		if item.Product != nil {
			name = item.Product.Name
		}
		if name == "" {
			name = item.Notes
		}
		if name == "" {
			name = "-"
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(colW[0], lineH, name, "1", 0, "L", false, 0, "")
		pdf.CellFormat(colW[1], lineH, fmt.Sprintf("%d", item.Quantity), "1", 0, "C", false, 0, "")
		pdf.CellFormat(colW[2], lineH, fmt.Sprintf("$%.2f", item.UnitPrice), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[3], lineH, fmt.Sprintf("$%.2f", item.TotalAmount), "1", 1, "R", false, 0, "")
		currentY += lineH
		totalAmount += item.TotalAmount
	}

	// 付款信息
	if currentY > pageH-margin-lineH*5 {
		pdf.AddPage()
		currentY = margin
	}
	paymentMethodText := ""
	if method, ok := paymentRecord["payment_method"].(string); ok {
		m := map[string]string{
			"cash":          "現金",
			"credit_card":   "信用卡",
			"bank_transfer": "銀行轉帳",
			"check":         "支票",
			"other":         "其他",
		}
		paymentMethodText = m[method]
		if paymentMethodText == "" {
			paymentMethodText = method
		}
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH, "支付方式：", "1", 0, "R", false, 0, "")
	pdf.CellFormat(colW[3], lineH, paymentMethodText, "1", 1, "L", false, 0, "")
	currentY += lineH

	ref := ""
	if r, ok := paymentRecord["reference_number"].(string); ok {
		ref = r
	}
	if ref != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH, "參考號碼：", "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[3], lineH, ref, "1", 1, "L", false, 0, "")
		currentY += lineH
	}

	amt := totalAmount
	if v, ok := paymentRecord["amount"].(float64); ok {
		amt = v
	}
	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH*1.2, "支出總額：", "1", 0, "R", false, 0, "")
	pdf.CellFormat(colW[3], lineH*1.2, fmt.Sprintf("$%.2f", amt), "1", 1, "R", false, 0, "")
	currentY += lineH * 1.5

	// 條款/備註
	if settings.Terms != "" {
		if currentY > pageH-margin-lineH*6 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "條款", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.MultiCell(usableW, lineH*0.8, settings.Terms, "", "L", false)
		currentY = pdf.GetY() + lineH*0.5
	}
	if settings.Notes != "" {
		if currentY > pageH-margin-lineH*4 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "備註", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.MultiCell(usableW, lineH*0.8, settings.Notes, "", "L", false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildServiceContractPDF 生成服務單合約 PDF
func buildServiceContractPDF(so models.ServiceOrder, settings models.DocumentSettings, enterprise models.Enterprise) ([]byte, error) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(36, 36, 36)
	pdf.SetAutoPageBreak(true, 36)

	// 設置中文字體
	fontName, hasBold := setupCJKFont(pdf)

	// 讀取全域文件樣式設定
	styleSettings := GetDocumentStyleSettings(settings.TenantID)
	titleSize := models.DefaultTitleFontSize
	subtitleSize := models.DefaultSubtitleFontSize
	bodySize := models.DefaultBodyFontSize
	sectionSize := models.DefaultSectionFontSize
	notesSize := models.DefaultNotesFontSize
	if styleSettings != nil {
		titleSize = styleSettings.GetTitleFontSize()
		bodySize = styleSettings.GetBodyFontSize()
		notesSize = styleSettings.GetNotesFontSize()
		subtitleSize = titleSize * 0.72
		sectionSize = bodySize + 2
	}

	pdf.AddPage()
	margin := 36.0
	pageW, pageH := pdf.GetPageSize()
	usableW := pageW - margin*2
	lineH := bodySize + 3
	currentY := margin

	// 標題（粗體）
	if hasBold {
		pdf.SetFont(fontName, "B", titleSize)
	} else {
		pdf.SetFont(fontName, "", titleSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH*1.5, "服務合約", "", 0, "C", false, 0, "")
	pdf.SetFont(fontName, "", subtitleSize)
	pdf.SetXY(margin, currentY+lineH*1.5)
	pdf.CellFormat(usableW, lineH, "SERVICE CONTRACT", "", 0, "C", false, 0, "")
	currentY += lineH * 3

	// 企業信息（使用文件 Logo）
	logoHeight, logoCleanup := drawDocumentLogo(pdf, enterprise, styleSettings, fontName, hasBold, margin, currentY, usableW, lineH)
	if logoCleanup != nil {
		defer logoCleanup()
	}
	currentY += logoHeight

	pdf.SetFont(fontName, "", bodySize)
	if addr := getEnterpriseAddress(enterprise); addr != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, addr, "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// 合約資訊
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("合約編號：%s", so.OrderNumber), "", 0, "L", false, 0, "")
	pdf.SetXY(margin+usableW/2, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("服務日期：%s", so.ServiceDate.Format("2006-01-02")), "", 1, "L", false, 0, "")
	currentY += lineH
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, fmt.Sprintf("狀態：%s", so.Status), "", 1, "L", false, 0, "")
	currentY += lineH * 1.5

	// 客戶資訊
	customerName := "散客"
	if so.Customer.ID != uuid.Nil {
		customerName = so.Customer.Name
		if so.Customer.LastName != "" {
			customerName = so.Customer.LastName + customerName
		}
	}
	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, "客戶資訊", "", 1, "L", false, 0, "")
	currentY += lineH
	pdf.SetFont(fontName, "", bodySize)
	pdf.CellFormat(usableW, lineH, fmt.Sprintf("客戶：%s", customerName), "", 1, "L", false, 0, "")
	currentY += lineH
	if so.ContactName != "" {
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("聯絡人：%s", so.ContactName), "", 1, "L", false, 0, "")
		currentY += lineH
	}
	if so.ContactAddress != "" {
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("地址：%s", so.ContactAddress), "", 1, "L", false, 0, "")
		currentY += lineH
	}
	if so.ContactPhone != "" {
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("電話：%s", so.ContactPhone), "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// 服務明細
	colW := []float64{usableW * 0.45, usableW * 0.15, usableW * 0.2, usableW * 0.2}
	if hasBold {
		pdf.SetFont(fontName, "B", bodySize)
	} else {
		pdf.SetFont(fontName, "", bodySize)
	}
	pdf.SetFillColor(240, 240, 240)
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0], lineH*1.2, "服務名稱", "1", 0, "L", true, 0, "")
	pdf.CellFormat(colW[1], lineH*1.2, "數量", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colW[2], lineH*1.2, "單價", "1", 0, "R", true, 0, "")
	pdf.CellFormat(colW[3], lineH*1.2, "小計", "1", 1, "R", true, 0, "")
	currentY += lineH * 1.2

	pdf.SetFont(fontName, "", bodySize)
	total := 0.0
	for _, item := range so.ServiceOrderItems {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		name := "服務"
		if item.Service != nil {
			name = item.Service.Name
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(colW[0], lineH, name, "1", 0, "L", false, 0, "")
		pdf.CellFormat(colW[1], lineH, fmt.Sprintf("%.2f", item.Quantity), "1", 0, "C", false, 0, "")
		pdf.CellFormat(colW[2], lineH, fmt.Sprintf("$%.2f", item.UnitPrice), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[3], lineH, fmt.Sprintf("$%.2f", item.TotalPrice), "1", 1, "R", false, 0, "")
		currentY += lineH
		total += item.TotalPrice
	}

	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(colW[0]+colW[1]+colW[2], lineH*1.2, "金額合計：", "1", 0, "R", false, 0, "")
	pdf.CellFormat(colW[3], lineH*1.2, fmt.Sprintf("$%.2f", total), "1", 1, "R", false, 0, "")
	currentY += lineH * 1.5

	// 條款 / 備註
	if settings.Terms != "" {
		if currentY > pageH-margin-lineH*6 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "條款", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.MultiCell(usableW, lineH*0.8, settings.Terms, "", "L", false)
		currentY = pdf.GetY() + lineH*0.5
	}
	if settings.Notes != "" {
		if currentY > pageH-margin-lineH*4 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "備註", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.MultiCell(usableW, lineH*0.8, settings.Notes, "", "L", false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// buildShippingNotePDF generates a PDF for a shipping note (發貨單).
// ---------------------------------------------------------------------------
func buildShippingNotePDF(
	order models.Order,
	shippingNote map[string]interface{},
	shippingNumber string,
	settings models.DocumentSettings,
	enterprise models.Enterprise,
	headers []string,
	rows [][]string,
) ([]byte, error) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(36, 36, 36)
	pdf.SetAutoPageBreak(true, 36)

	fontName, hasBold := setupCJKFont(pdf)

	// 讀取全域文件樣式設定
	styleSettings := GetDocumentStyleSettings(settings.TenantID)
	titleSize := models.DefaultTitleFontSize
	subtitleSize := models.DefaultSubtitleFontSize
	bodySize := models.DefaultBodyFontSize
	sectionSize := models.DefaultSectionFontSize
	notesSize := models.DefaultNotesFontSize
	if styleSettings != nil {
		titleSize = styleSettings.GetTitleFontSize()
		bodySize = styleSettings.GetBodyFontSize()
		notesSize = styleSettings.GetNotesFontSize()
		subtitleSize = titleSize * 0.72
		sectionSize = bodySize + 2
	}

	pdf.AddPage()
	margin := 36.0
	pageW, pageH := pdf.GetPageSize()
	usableW := pageW - margin*2
	lineH := bodySize + 3
	currentY := margin

	// ---- Title（粗體）----
	if hasBold {
		pdf.SetFont(fontName, "B", titleSize)
	} else {
		pdf.SetFont(fontName, "", titleSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH*1.5, "發貨單", "", 0, "C", false, 0, "")
	pdf.SetFont(fontName, "", subtitleSize)
	pdf.SetXY(margin, currentY+lineH*1.5)
	pdf.CellFormat(usableW, lineH, "SHIPPING NOTE", "", 0, "C", false, 0, "")
	currentY += lineH * 3

	// ---- Enterprise logo / name（使用文件 Logo）----
	logoHeight, logoCleanup := drawDocumentLogo(pdf, enterprise, styleSettings, fontName, hasBold, margin, currentY, usableW, lineH)
	if logoCleanup != nil {
		defer logoCleanup()
	}
	currentY += logoHeight

	pdf.SetFont(fontName, "", bodySize)
	enterpriseAddr := getEnterpriseAddress(enterprise)
	if enterpriseAddr != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, enterpriseAddr, "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// ---- Shipping info ----
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("發貨單號：%s", shippingNumber), "", 0, "L", false, 0, "")

	shippingDate := ""
	if d, ok := shippingNote["shipping_date"].(string); ok {
		shippingDate = d
	}
	pdf.SetXY(margin+usableW/2, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("發貨日期：%s", shippingDate), "", 1, "L", false, 0, "")
	currentY += lineH

	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, fmt.Sprintf("訂單編號：%s", order.OrderNumber), "", 1, "L", false, 0, "")
	currentY += lineH

	// Logistics info
	if company, ok := shippingNote["shipping_company"].(string); ok && company != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("物流公司：%s", company), "", 0, "L", false, 0, "")
		if tracking, ok2 := shippingNote["tracking_number"].(string); ok2 && tracking != "" {
			pdf.SetXY(margin+usableW/2, currentY)
			pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("追蹤號碼：%s", tracking), "", 1, "L", false, 0, "")
		}
		currentY += lineH
	}
	currentY += lineH * 0.5

	// ---- Recipient info ----
	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, "收件人信息", "", 1, "L", false, 0, "")
	currentY += lineH

	pdf.SetFont(fontName, "", bodySize)
	contactName := order.ContactName
	if contactName == "" && order.Customer != nil {
		contactName = order.Customer.Name
	}
	if contactName != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("收件人：%s", contactName), "", 1, "L", false, 0, "")
		currentY += lineH
	}

	shippingAddr := ""
	if addr, ok := shippingNote["shipping_address"].(string); ok && addr != "" {
		shippingAddr = addr
	} else if order.ContactAddress != "" {
		shippingAddr = order.ContactAddress
	} else if order.Customer != nil {
		shippingAddr = order.Customer.Address
	}
	if shippingAddr != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("地址：%s", shippingAddr), "", 1, "L", false, 0, "")
		currentY += lineH
	}

	contactPhone := order.ContactPhone
	if contactPhone == "" && order.Customer != nil {
		contactPhone = order.Customer.Phone
	}
	if contactPhone != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("電話：%s", contactPhone), "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// ---- Items table ----
	numCols := len(headers)
	colW := make([]float64, numCols)
	if numCols == 2 {
		colW[0] = usableW * 0.7
		colW[1] = usableW * 0.3
	} else {
		each := usableW / float64(numCols)
		for i := range colW {
			colW[i] = each
		}
	}

	// Table header
	if hasBold {
		pdf.SetFont(fontName, "B", bodySize)
	} else {
		pdf.SetFont(fontName, "", bodySize)
	}
	pdf.SetFillColor(240, 240, 240)
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetXY(margin, currentY)
	for i, h := range headers {
		align := "L"
		if i == numCols-1 {
			align = "C"
		}
		ln := 0
		if i == numCols-1 {
			ln = 1
		}
		pdf.CellFormat(colW[i], lineH*1.2, h, "1", ln, align, true, 0, "")
	}
	currentY += lineH * 1.2

	// Table rows
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetFillColor(255, 255, 255)
	for _, row := range rows {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		pdf.SetXY(margin, currentY)
		for i, cell := range row {
			if i >= numCols {
				break
			}
			align := "L"
			if i == numCols-1 {
				align = "C"
			}
			ln := 0
			if i == numCols-1 {
				ln = 1
			}
			pdf.CellFormat(colW[i], lineH, cell, "1", ln, align, false, 0, "")
		}
		currentY += lineH
	}
	currentY += lineH

	// ---- Terms ----
	if settings.Terms != "" {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "條款", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.MultiCell(usableW, lineH*0.8, settings.Terms, "", "L", false)
		currentY = pdf.GetY() + lineH*0.5
	}

	// ---- Notes ----
	if settings.Notes != "" {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "備註", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.MultiCell(usableW, lineH*0.8, settings.Notes, "", "L", false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// buildReceivingNotePDF generates a PDF for a receiving note (收貨單).
// ---------------------------------------------------------------------------
func buildReceivingNotePDF(
	po models.PurchaseOrder,
	receivingNote map[string]interface{},
	receivingNumber string,
	enterprise models.Enterprise,
	tenantID uuid.UUID,
	headers []string,
	rows [][]string,
) ([]byte, error) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(36, 36, 36)
	pdf.SetAutoPageBreak(true, 36)

	fontName, hasBold := setupCJKFont(pdf)

	// 讀取全域文件樣式設定
	styleSettings := GetDocumentStyleSettings(tenantID)
	titleSize := models.DefaultTitleFontSize
	subtitleSize := models.DefaultSubtitleFontSize
	bodySize := models.DefaultBodyFontSize
	sectionSize := models.DefaultSectionFontSize
	notesSize := models.DefaultNotesFontSize
	if styleSettings != nil {
		titleSize = styleSettings.GetTitleFontSize()
		bodySize = styleSettings.GetBodyFontSize()
		notesSize = styleSettings.GetNotesFontSize()
		subtitleSize = titleSize * 0.72
		sectionSize = bodySize + 2
	}

	pdf.AddPage()
	margin := 36.0
	pageW, pageH := pdf.GetPageSize()
	usableW := pageW - margin*2
	lineH := bodySize + 3
	currentY := margin

	// ---- Title（粗體）----
	if hasBold {
		pdf.SetFont(fontName, "B", titleSize)
	} else {
		pdf.SetFont(fontName, "", titleSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH*1.5, "收貨單", "", 0, "C", false, 0, "")
	pdf.SetFont(fontName, "", subtitleSize)
	pdf.SetXY(margin, currentY+lineH*1.5)
	pdf.CellFormat(usableW, lineH, "RECEIVING NOTE", "", 0, "C", false, 0, "")
	currentY += lineH * 3

	// ---- Enterprise logo / name（使用文件 Logo）----
	logoHeight, logoCleanup := drawDocumentLogo(pdf, enterprise, styleSettings, fontName, hasBold, margin, currentY, usableW, lineH)
	if logoCleanup != nil {
		defer logoCleanup()
	}
	currentY += logoHeight

	pdf.SetFont(fontName, "", bodySize)
	enterpriseAddr := getEnterpriseAddress(enterprise)
	if enterpriseAddr != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, enterpriseAddr, "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// ---- Receiving info ----
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("收貨單號：%s", receivingNumber), "", 0, "L", false, 0, "")

	receivingDate := ""
	if d, ok := receivingNote["receiving_date"].(string); ok {
		receivingDate = d
	}
	pdf.SetXY(margin+usableW/2, currentY)
	pdf.CellFormat(usableW/2, lineH, fmt.Sprintf("收貨日期：%s", receivingDate), "", 1, "L", false, 0, "")
	currentY += lineH

	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, fmt.Sprintf("採購單號：%s", po.OrderNumber), "", 1, "L", false, 0, "")
	currentY += lineH * 1.5

	// ---- Supplier info ----
	if hasBold {
		pdf.SetFont(fontName, "B", sectionSize)
	} else {
		pdf.SetFont(fontName, "", sectionSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, "供應商信息", "", 1, "L", false, 0, "")
	currentY += lineH

	pdf.SetFont(fontName, "", bodySize)
	if po.Supplier != nil {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, fmt.Sprintf("供應商：%s", po.Supplier.Name), "", 1, "L", false, 0, "")
		currentY += lineH

		if po.Supplier.Email != "" {
			pdf.SetXY(margin, currentY)
			pdf.CellFormat(usableW, lineH, fmt.Sprintf("聯絡人：%s", po.Supplier.Email), "", 1, "L", false, 0, "")
			currentY += lineH
		}
		if po.Supplier.Phone != "" {
			pdf.SetXY(margin, currentY)
			pdf.CellFormat(usableW, lineH, fmt.Sprintf("電話：%s", po.Supplier.Phone), "", 1, "L", false, 0, "")
			currentY += lineH
		}
		if po.Supplier.Address != "" {
			pdf.SetXY(margin, currentY)
			pdf.CellFormat(usableW, lineH, fmt.Sprintf("地址：%s", po.Supplier.Address), "", 1, "L", false, 0, "")
			currentY += lineH
		}
	}
	currentY += lineH * 0.5

	// ---- Items table ----
	numCols := len(headers)
	colW := make([]float64, numCols)
	if numCols == 2 {
		colW[0] = usableW * 0.7
		colW[1] = usableW * 0.3
	} else {
		each := usableW / float64(numCols)
		for i := range colW {
			colW[i] = each
		}
	}

	// Table header
	if hasBold {
		pdf.SetFont(fontName, "B", bodySize)
	} else {
		pdf.SetFont(fontName, "", bodySize)
	}
	pdf.SetFillColor(240, 240, 240)
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetXY(margin, currentY)
	for i, h := range headers {
		align := "L"
		if i == numCols-1 {
			align = "C"
		}
		ln := 0
		if i == numCols-1 {
			ln = 1
		}
		pdf.CellFormat(colW[i], lineH*1.2, h, "1", ln, align, true, 0, "")
	}
	currentY += lineH * 1.2

	// Table rows
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetFillColor(255, 255, 255)
	for _, row := range rows {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		pdf.SetXY(margin, currentY)
		for i, cell := range row {
			if i >= numCols {
				break
			}
			align := "L"
			if i == numCols-1 {
				align = "C"
			}
			ln := 0
			if i == numCols-1 {
				ln = 1
			}
			pdf.CellFormat(colW[i], lineH, cell, "1", ln, align, false, 0, "")
		}
		currentY += lineH
	}

	// ---- Notes from receivingNote ----
	if notes, ok := receivingNote["notes"].(string); ok && notes != "" {
		currentY += lineH
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		if hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, "備註", "", 1, "L", false, 0, "")
		currentY += lineH
		pdf.SetFont(fontName, "", notesSize)
		pdf.MultiCell(usableW, lineH*0.8, notes, "", "L", false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// buildRefundNotePDF generates a PDF for a refund note (退款單).
// Shared by both order refunds and service-order refunds.
// The title is pre-formatted by the caller (e.g. "退款單 #1（訂單 ORD-001 / 客戶名）").
// The rows already include the "合計" total row and optional "額外款項" row.
// ---------------------------------------------------------------------------
func buildRefundNotePDF(
	title string,
	enterprise models.Enterprise,
	tenantID uuid.UUID,
	headers []string,
	rows [][]string,
) ([]byte, error) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(36, 36, 36)
	pdf.SetAutoPageBreak(true, 36)

	fontName, hasBold := setupCJKFont(pdf)

	// 讀取全域文件樣式設定
	styleSettings := GetDocumentStyleSettings(tenantID)
	titleSize := models.DefaultTitleFontSize
	subtitleSize := models.DefaultSubtitleFontSize
	bodySize := models.DefaultBodyFontSize
	sectionSize := models.DefaultSectionFontSize
	notesSize := models.DefaultNotesFontSize
	if styleSettings != nil {
		titleSize = styleSettings.GetTitleFontSize()
		bodySize = styleSettings.GetBodyFontSize()
		notesSize = styleSettings.GetNotesFontSize()
		subtitleSize = titleSize * 0.72
		sectionSize = bodySize + 2
	}
	_ = sectionSize
	_ = notesSize

	pdf.AddPage()
	margin := 36.0
	pageW, pageH := pdf.GetPageSize()
	usableW := pageW - margin*2
	lineH := bodySize + 3
	currentY := margin

	// ---- Title（粗體）----
	if hasBold {
		pdf.SetFont(fontName, "B", titleSize)
	} else {
		pdf.SetFont(fontName, "", titleSize)
	}
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH*1.5, "退款單", "", 0, "C", false, 0, "")
	pdf.SetFont(fontName, "", subtitleSize)
	pdf.SetXY(margin, currentY+lineH*1.5)
	pdf.CellFormat(usableW, lineH, "REFUND NOTE", "", 0, "C", false, 0, "")
	currentY += lineH * 3

	// ---- Enterprise logo / name（使用文件 Logo）----
	logoHeight, logoCleanup := drawDocumentLogo(pdf, enterprise, styleSettings, fontName, hasBold, margin, currentY, usableW, lineH)
	if logoCleanup != nil {
		defer logoCleanup()
	}
	currentY += logoHeight

	pdf.SetFont(fontName, "", bodySize)
	enterpriseAddr := getEnterpriseAddress(enterprise)
	if enterpriseAddr != "" {
		pdf.SetXY(margin, currentY)
		pdf.CellFormat(usableW, lineH, enterpriseAddr, "", 1, "L", false, 0, "")
		currentY += lineH
	}
	currentY += lineH * 0.5

	// ---- Subtitle (contains refund number, order number, customer) ----
	pdf.SetFont(fontName, "", bodySize)
	pdf.SetXY(margin, currentY)
	pdf.CellFormat(usableW, lineH, title, "", 1, "L", false, 0, "")
	currentY += lineH * 1.5

	// ---- Items table ----
	numCols := len(headers)
	colW := make([]float64, numCols)
	if numCols == 4 {
		// 產品/服務, 數量, 單價, 小計
		colW[0] = usableW * 0.4
		colW[1] = usableW * 0.15
		colW[2] = usableW * 0.2
		colW[3] = usableW * 0.25
	} else {
		each := usableW / float64(numCols)
		for i := range colW {
			colW[i] = each
		}
	}

	// Column alignments for refund tables
	colAlign := make([]string, numCols)
	if numCols == 4 {
		colAlign[0] = "L"
		colAlign[1] = "C"
		colAlign[2] = "R"
		colAlign[3] = "R"
	} else {
		for i := range colAlign {
			colAlign[i] = "L"
		}
	}

	// Table header
	if hasBold {
		pdf.SetFont(fontName, "B", bodySize)
	} else {
		pdf.SetFont(fontName, "", bodySize)
	}
	pdf.SetFillColor(240, 240, 240)
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetXY(margin, currentY)
	for i, h := range headers {
		ln := 0
		if i == numCols-1 {
			ln = 1
		}
		pdf.CellFormat(colW[i], lineH*1.2, h, "1", ln, colAlign[i], true, 0, "")
	}
	currentY += lineH * 1.2

	// Table rows — last row (合計) is rendered in bold
	pdf.SetFillColor(255, 255, 255)
	for rowIdx, row := range rows {
		if currentY > pageH-margin-lineH*5 {
			pdf.AddPage()
			currentY = margin
		}
		isLastRow := rowIdx == len(rows)-1
		if isLastRow && hasBold {
			pdf.SetFont(fontName, "B", bodySize)
		} else {
			pdf.SetFont(fontName, "", bodySize)
		}
		pdf.SetXY(margin, currentY)
		for i, cell := range row {
			if i >= numCols {
				break
			}
			ln := 0
			if i == numCols-1 {
				ln = 1
			}
			pdf.CellFormat(colW[i], lineH, cell, "1", ln, colAlign[i], false, 0, "")
		}
		currentY += lineH
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
