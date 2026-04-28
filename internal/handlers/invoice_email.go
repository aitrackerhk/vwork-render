package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// SendInvoiceEmail 將付款記錄的發票作為附件發送到客戶電郵
func SendInvoiceEmail(c *fiber.Ctx) error {
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

	// 從 income 表獲取付款記錄
	var incomes []models.Income
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "order", orderID).Find(&incomes)

	if recordIndex < 0 || recordIndex >= len(incomes) {
		return c.Status(404).JSON(fiber.Map{"error": "Payment record not found"})
	}

	inc := incomes[recordIndex]
	ef := map[string]interface{}{}
	if inc.ExtraFields != nil {
		ef = map[string]interface{}(inc.ExtraFields)
	}

	invoiceNumber, _ := ef["invoice_number"].(string)
	if invoiceNumber == "" {
		// 如果沒有發票號碼，嘗試即時生成一個（通常應該已經有了）
		today := time.Now().Format("20060102")
		invoiceNumber = fmt.Sprintf("INV-%s-%03d", today, recordIndex+1)
	}

	// 獲取發票設定
	var invoiceSettings models.DocumentSettings
	if err := database.DB.Where("tenant_id = ? AND document_type = ?", tenantID, "invoice").First(&invoiceSettings).Error; err != nil {
		invoiceSettings = models.DocumentSettings{}
	}

	// 獲取企業信息
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	// 生成 PDF
	paymentRecordMap := map[string]interface{}{
		"payment_date":     inc.IncomeDate.Format("2006-01-02"),
		"payment_method":   inc.PaymentMethod,
		"amount":           inc.Amount,
		"invoice_number":   invoiceNumber,
		"reference_number": ef["reference_number"],
		"notes":            inc.Notes,
	}

	pdfBytes, err := buildInvoicePDF(order, paymentRecordMap, invoiceNumber, invoiceSettings, enterprise, false)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate invoice PDF: " + err.Error()})
	}

	// 確定收件人電郵
	toEmail := order.ContactEmail
	if toEmail == "" && order.Customer != nil {
		toEmail = order.Customer.Email
	}
	if toEmail == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Customer email is missing"})
	}

	toName := order.ContactName
	if toName == "" && order.Customer != nil {
		toName = order.Customer.Name
	}

	// 獲取租戶資訊（用於 subdomain 和 branding）
	var tenant models.Tenant
	database.DB.Where("id = ?", tenantID).First(&tenant)

	// 獲取語言
	lang := "zh-hant"
	if l, ok := c.Locals("lang").(string); ok {
		lang = l
	}

	// 獲取 Branding
	b := email.TenantBranding(tenantID)
	b.LogoURL = email.PublicAssetURL(b.LogoURL)

	// 生成郵件內容
	subject, textBody, htmlBody, err := email.InvoiceEmail(b, tenant.Subdomain, toName, invoiceNumber, inc.Amount, lang)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate email content: " + err.Error()})
	}

	// 建立郵件任務
	job := models.EmailJob{
		TenantID: &tenantID,
		Kind:     "invoice",
		ToEmail:  toEmail,
		Subject:  subject,
		BodyText: textBody,
		BodyHTML: htmlBody,
		Attachments: models.EmailAttachmentsJSONB{
			{
				Name:        fmt.Sprintf("invoice_%s.pdf", invoiceNumber),
				ContentType: "application/pdf",
				Data:        pdfBytes,
			},
		},
		Status:    "queued",
		RunAt:     time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := database.DB.Create(&job).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to queue email: " + err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Invoice email queued successfully"})
}
