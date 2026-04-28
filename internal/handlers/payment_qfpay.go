package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/utils"
)

// ===== QFPay Payment Handlers =====
// Unified handler for all QFPay-based payment methods:
//   FPS (轉數快), PayMe, Alipay HK, WeChat Pay HK, BoC Pay, Octopus
//
// Flow:
//   1. Frontend calls create-payment → backend calls QFPay API → returns pay_url/qr_code
//   2. Customer redirects to wallet app or scans QR code
//   3. QFPay sends webhook to /api/v1/webhooks/qfpay/notify
//   4. Backend verifies signature, marks order completed, creates income
//   5. Frontend polls /check-status to show success

// ──────────────────────────────────────────────────────────────
// Webstore Public Checkout — QFPay Methods
// ──────────────────────────────────────────────────────────────

// PublicCreateQFPayPayment creates an Order + QFPay payment for FPS/PayMe/etc.
// POST /api/v1/public/:subdomain/checkout/qfpay/create-payment
func PublicCreateQFPayPayment(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	customerID, err := getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var req checkoutCreateReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	pmID, err := uuid.Parse(strings.TrimSpace(req.PaymentMethodID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payment_method_id"})
	}

	var pm models.PaymentMethod
	if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ? AND is_online_payment = ?",
		pmID, tenant.ID, "active", true).First(&pm).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "payment method not found"})
	}

	code := strings.ToLower(strings.TrimSpace(pm.Code))
	if !IsQFPayMethod(code) {
		return c.Status(400).JSON(fiber.Map{"error": "payment method is not a QFPay method"})
	}

	payType := QFPayPayType(code)
	if payType == "" {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported QFPay pay_type"})
	}

	// Resolve QFPay credentials
	extraFields := map[string]interface{}(pm.ExtraFields)
	creds := resolveQFPayCredentials(code, extraFields)
	if creds == nil {
		return c.Status(500).JSON(fiber.Map{"error": "QFPay credentials not configured"})
	}

	// Build order from cart
	order, orderItems, _, err := buildOrderFromCart(tenant.ID, customerID, req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Persist customer language
	persistCustomerLanguage(tenant.ID, customerID, req.Lang, c.Query("lang", ""))

	// Create order + items in DB
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if err := tx.Create(order).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "failed to create order"})
	}
	for i := range orderItems {
		orderItems[i].OrderID = order.ID
		if err := tx.Create(&orderItems[i]).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "failed to create order items"})
		}
	}

	// Amount in cents
	amountCents := int64(order.TotalAmount * 100)
	if amountCents <= 0 {
		tx.Rollback()
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount"})
	}

	// Build notify URL
	cfg := mustAppConfig()
	notifyURL := cfg.QFPay.NotifyURL
	if notifyURL == "" {
		scheme := c.Protocol()
		host := c.Hostname()
		notifyURL = scheme + "://" + host + "/api/v1/webhooks/qfpay/notify"
	}

	// Call QFPay API
	goodsName := fmt.Sprintf("Order %s", order.OrderNumber)
	qfResp, err := qfpayCreatePayment(creds, payType, amountCents, "HKD", order.ID.String(), notifyURL, goodsName)
	if err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("QFPay payment creation failed: %v", err)})
	}

	// Store payment info
	ef := map[string]interface{}(order.ExtraFields)
	ef["payment_provider"] = "qfpay"
	ef["payment_gateway"] = code
	ef["payment_method_id"] = pm.ID.String()
	ef["qfpay_order_id"] = qfResp.OrderID
	ef["qfpay_pay_type"] = payType
	ef["currency"] = "HKD"
	order.ExtraFields = models.JSONB(ef)
	if err := tx.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).
		Update("extra_fields", order.ExtraFields).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "failed to update order payment info"})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "transaction failed"})
	}

	return c.JSON(fiber.Map{
		"order_id":       order.ID.String(),
		"order_number":   order.OrderNumber,
		"amount":         order.TotalAmount,
		"currency":       "HKD",
		"gateway":        code,
		"qfpay_order_id": qfResp.OrderID,
		"qr_code":        qfResp.QRCode,
		"pay_url":        qfResp.PayURL,
	})
}

// PublicCheckQFPayStatus polls QFPay for payment completion status.
// POST /api/v1/public/:subdomain/checkout/qfpay/check-status
func PublicCheckQFPayStatus(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	_, err = getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var req struct {
		OrderID      string `json:"order_id"`
		QFPayOrderID string `json:"qfpay_order_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	oid, err := uuid.Parse(strings.TrimSpace(req.OrderID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid order_id"})
	}

	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", oid, tenant.ID).First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "order not found"})
	}

	// Already completed?
	if order.Status == "completed" {
		return c.JSON(fiber.Map{"ok": true, "status": "completed", "order_id": order.ID.String()})
	}

	// Find QFPay credentials
	pmID := ""
	if ef := getOrderExtraMap(&order); ef != nil {
		if v, ok := ef["payment_method_id"].(string); ok {
			pmID = v
		}
	}
	var pm *models.PaymentMethod
	if pmID != "" {
		pm, _ = loadPaymentMethodByID(tenant.ID, pmID)
	}

	var extraFields map[string]interface{}
	if pm != nil {
		extraFields = map[string]interface{}(pm.ExtraFields)
	}
	creds := resolveQFPayCredentials("", extraFields)
	if creds == nil {
		return c.Status(500).JSON(fiber.Map{"error": "QFPay not configured"})
	}

	// Query QFPay
	qfOrderID := strings.TrimSpace(req.QFPayOrderID)
	if qfOrderID == "" {
		// Get from order extra_fields
		if ef := getOrderExtraMap(&order); ef != nil {
			if v, ok := ef["qfpay_order_id"].(string); ok {
				qfOrderID = v
			}
		}
	}
	if qfOrderID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing qfpay_order_id"})
	}

	qfResp, err := qfpayQueryOrder(creds, qfOrderID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("QFPay query failed: %v", err)})
	}

	// Check if any transaction succeeded
	for _, d := range qfResp.Data {
		if d.Status == "1" { // success
			// Mark order completed
			if err := completeQFPayOrder(tenant.ID, &order, pm, qfOrderID); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "failed to complete order"})
			}
			return c.JSON(fiber.Map{"ok": true, "status": "completed", "order_id": order.ID.String()})
		}
	}

	return c.JSON(fiber.Map{"ok": true, "status": "pending", "order_id": order.ID.String()})
}

// ──────────────────────────────────────────────────────────────
// Payment Link — QFPay Methods
// ──────────────────────────────────────────────────────────────

// PaymentLinkCreateQFPayPayment creates a QFPay payment for a payment link.
// POST /api/v1/pay/:token/qfpay/create-payment
func PaymentLinkCreateQFPayPayment(c *fiber.Ctx) error {
	token := c.Params("token")
	link, tenant, order, err := loadPaymentLinkByToken(token)
	if err != nil {
		status := 400
		if link == nil {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	var req struct {
		Gateway string `json:"gateway"` // e.g. "fps", "payme", "alipay_hk", etc.
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	code := strings.ToLower(strings.TrimSpace(req.Gateway))
	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing gateway code"})
	}
	if !IsQFPayMethod(code) {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported QFPay gateway: " + code})
	}

	payType := QFPayPayType(code)
	if payType == "" {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported QFPay pay_type"})
	}

	// Find payment method
	var pm models.PaymentMethod
	if err := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code = ?",
		tenant.ID, "active", true, code).First(&pm).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": code + " payment method not configured"})
	}

	extraFields := map[string]interface{}(pm.ExtraFields)
	creds := resolveQFPayCredentials(code, extraFields)
	if creds == nil {
		return c.Status(500).JSON(fiber.Map{"error": "QFPay credentials not configured"})
	}

	amountCents := int64(order.TotalAmount * 100)
	if amountCents <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount"})
	}

	// Build notify URL
	cfg := mustAppConfig()
	notifyURL := cfg.QFPay.NotifyURL
	if notifyURL == "" {
		scheme := c.Protocol()
		host := c.Hostname()
		notifyURL = scheme + "://" + host + "/api/v1/webhooks/qfpay/notify"
	}

	goodsName := fmt.Sprintf("Order %s", order.OrderNumber)
	qfResp, err := qfpayCreatePayment(creds, payType, amountCents, "HKD", order.ID.String(), notifyURL, goodsName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("QFPay payment creation failed: %v", err)})
	}

	// Store payment info
	ef := getOrderExtraMap(order)
	ef["payment_provider"] = "qfpay"
	ef["payment_gateway"] = code
	ef["payment_method_id"] = pm.ID.String()
	ef["qfpay_order_id"] = qfResp.OrderID
	ef["qfpay_pay_type"] = payType
	ef["payment_link_token"] = token
	ef["currency"] = "HKD"
	database.DB.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).
		Update("extra_fields", models.JSONB(ef))

	return c.JSON(fiber.Map{
		"order_id":       order.ID.String(),
		"order_number":   order.OrderNumber,
		"amount":         order.TotalAmount,
		"currency":       "HKD",
		"gateway":        code,
		"qfpay_order_id": qfResp.OrderID,
		"qr_code":        qfResp.QRCode,
		"pay_url":        qfResp.PayURL,
	})
}

// PaymentLinkCheckQFPayStatus polls QFPay for payment link payment status.
// POST /api/v1/pay/:token/qfpay/check-status
func PaymentLinkCheckQFPayStatus(c *fiber.Ctx) error {
	token := c.Params("token")
	link, tenant, order, err := loadPaymentLinkByToken(token)
	if err != nil {
		status := 400
		if link == nil {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	// Already completed?
	if order.Status == "completed" {
		return c.JSON(fiber.Map{"ok": true, "status": "completed", "order_id": order.ID.String()})
	}

	// Get QFPay order ID from order extra_fields
	qfOrderID := ""
	pmID := ""
	if ef := getOrderExtraMap(order); ef != nil {
		if v, ok := ef["qfpay_order_id"].(string); ok {
			qfOrderID = v
		}
		if v, ok := ef["payment_method_id"].(string); ok {
			pmID = v
		}
	}
	if qfOrderID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "no QFPay order found"})
	}

	var pm *models.PaymentMethod
	if pmID != "" {
		pm, _ = loadPaymentMethodByID(tenant.ID, pmID)
	}

	var extraFields map[string]interface{}
	if pm != nil {
		extraFields = map[string]interface{}(pm.ExtraFields)
	}
	creds := resolveQFPayCredentials("", extraFields)
	if creds == nil {
		return c.Status(500).JSON(fiber.Map{"error": "QFPay not configured"})
	}

	qfResp, err := qfpayQueryOrder(creds, qfOrderID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("QFPay query failed: %v", err)})
	}

	for _, d := range qfResp.Data {
		if d.Status == "1" {
			if err := completeQFPayOrder(tenant.ID, order, pm, qfOrderID); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "failed to complete order"})
			}
			// Also mark payment link as paid
			database.DB.Model(&models.PaymentLink{}).Where("id = ?", link.ID).Updates(map[string]interface{}{
				"status":     "paid",
				"updated_at": time.Now(),
			})
			return c.JSON(fiber.Map{"ok": true, "status": "completed", "order_id": order.ID.String()})
		}
	}

	return c.JSON(fiber.Map{"ok": true, "status": "pending", "order_id": order.ID.String()})
}

// ──────────────────────────────────────────────────────────────
// QFPay Webhook
// ──────────────────────────────────────────────────────────────

// QFPayWebhookNotify handles QFPay payment completion webhooks.
// POST /api/v1/webhooks/qfpay/notify
func QFPayWebhookNotify(c *fiber.Ctx) error {
	// Parse form/JSON body
	notification := make(map[string]string)

	// QFPay sends form-encoded notifications
	c.Request().PostArgs().VisitAll(func(key, value []byte) {
		notification[string(key)] = string(value)
	})

	// If empty, try JSON
	if len(notification) == 0 {
		var jsonNotif map[string]string
		if err := c.BodyParser(&jsonNotif); err == nil {
			notification = jsonNotif
		}
	}

	if len(notification) == 0 {
		return c.Status(400).SendString("invalid notification")
	}

	// Get order ID (out_trade_no is our order ID)
	orderIDStr := notification["out_trade_no"]
	if orderIDStr == "" {
		return c.Status(400).SendString("missing out_trade_no")
	}

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return c.Status(400).SendString("invalid out_trade_no")
	}

	var order models.Order
	if err := database.DB.Where("id = ?", orderID).First(&order).Error; err != nil {
		return c.Status(404).SendString("order not found")
	}

	// Already completed — return success to prevent retries
	if order.Status == "completed" {
		return c.SendString("SUCCESS")
	}

	// Get payment method from order extra_fields
	pmID := ""
	if ef := getOrderExtraMap(&order); ef != nil {
		if v, ok := ef["payment_method_id"].(string); ok {
			pmID = v
		}
	}
	var pm *models.PaymentMethod
	if pmID != "" {
		pm, _ = loadPaymentMethodByID(order.TenantID, pmID)
	}

	// Verify signature
	var extraFields map[string]interface{}
	if pm != nil {
		extraFields = map[string]interface{}(pm.ExtraFields)
	}
	creds := resolveQFPayCredentials("", extraFields)
	if creds != nil {
		if !qfpayVerifyNotification(notification, creds.ClientKey) {
			return c.Status(403).SendString("signature verification failed")
		}
	}

	// Check status: respcd "0000" and status "1" means success
	respCode := notification["respcd"]
	status := notification["status"]
	if respCode != "0000" || status != "1" {
		// Payment failed or pending — just acknowledge
		return c.SendString("SUCCESS")
	}

	syssn := notification["syssn"]
	if err := completeQFPayOrder(order.TenantID, &order, pm, syssn); err != nil {
		// Log error but still return SUCCESS to prevent retries
		return c.SendString("SUCCESS")
	}

	// Also mark any payment link as paid
	database.DB.Model(&models.PaymentLink{}).
		Where("order_id = ? AND tenant_id = ? AND status = ?", order.ID, order.TenantID, "active").
		Updates(map[string]interface{}{
			"status":     "paid",
			"updated_at": time.Now(),
		})

	return c.SendString("SUCCESS")
}

// ──────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────

// completeQFPayOrder marks an order as completed and creates the income record.
func completeQFPayOrder(tenantID uuid.UUID, order *models.Order, pm *models.PaymentMethod, referenceNumber string) error {
	if order.Status == "completed" {
		return nil // idempotent
	}

	tx := database.DB.Begin()
	if err := tx.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenantID).Updates(map[string]interface{}{
		"status":     "completed",
		"updated_at": utils.NowInTenantTimezone(tenantID),
	}).Error; err != nil {
		tx.Rollback()
		return err
	}

	gatewayCode := ""
	if ef := getOrderExtraMap(order); ef != nil {
		if v, ok := ef["payment_gateway"].(string); ok {
			gatewayCode = v
		}
	}
	providerLabel := "qfpay"
	if gatewayCode != "" {
		providerLabel = gatewayCode
	}

	if pm != nil {
		_ = createIncomeForPaidOrder(tenantID, order, pm, providerLabel, referenceNumber)
	}

	return tx.Commit().Error
}
