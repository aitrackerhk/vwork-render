package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"nwork/internal/database"
	"nwork/internal/models"
)

// ===== UnionPay (銀聯/雲閃付) Payment Handlers =====
//
// UnionPay integration strategy:
//   - Primary: Route through QFPay (if QFPay credentials available)
//   - Future:  Direct UnionPay Online Payment API
//
// QFPay UnionPay pay_type codes:
//   - 800201: UnionPay QR Code (QuickPass/雲閃付)
//
// Flow is identical to other QFPay methods: create → redirect/QR → webhook → complete

const qfpayUnionPayPayType = "800201" // UnionPay QuickPass Online

// ──────────────────────────────────────────────────────────────
// Webstore Public Checkout — UnionPay
// ──────────────────────────────────────────────────────────────

// PublicCreateUnionPayPayment creates an Order + UnionPay payment.
// POST /api/v1/public/:subdomain/checkout/unionpay/create-payment
func PublicCreateUnionPayPayment(c *fiber.Ctx) error {
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
	if code != GatewayUnionPay {
		return c.Status(400).JSON(fiber.Map{"error": "payment method is not UnionPay"})
	}

	// Resolve QFPay credentials (UnionPay routes through QFPay)
	extraFields := map[string]interface{}(pm.ExtraFields)
	creds := resolveQFPayCredentials(code, extraFields)
	if creds == nil {
		return c.Status(500).JSON(fiber.Map{"error": "UnionPay (QFPay) credentials not configured"})
	}

	// Determine currency (UnionPay supports HKD and CNY)
	currency := strings.ToLower(parseExtraString(pm.ExtraFields, "currency"))
	if currency == "" {
		currency = "hkd"
	}

	// Build order
	order, orderItems, _, err := buildOrderFromCart(tenant.ID, customerID, req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	persistCustomerLanguage(tenant.ID, customerID, req.Lang, c.Query("lang", ""))

	// Create order + items
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

	goodsName := fmt.Sprintf("Order %s", order.OrderNumber)
	qfResp, err := qfpayCreatePayment(creds, qfpayUnionPayPayType, amountCents, strings.ToUpper(currency), order.ID.String(), notifyURL, goodsName)
	if err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("UnionPay payment creation failed: %v", err)})
	}

	// Store payment info
	ef := map[string]interface{}(order.ExtraFields)
	ef["payment_provider"] = "qfpay"
	ef["payment_gateway"] = GatewayUnionPay
	ef["payment_method_id"] = pm.ID.String()
	ef["qfpay_order_id"] = qfResp.OrderID
	ef["qfpay_pay_type"] = qfpayUnionPayPayType
	ef["currency"] = strings.ToUpper(currency)
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
		"currency":       strings.ToUpper(currency),
		"gateway":        GatewayUnionPay,
		"qfpay_order_id": qfResp.OrderID,
		"qr_code":        qfResp.QRCode,
		"pay_url":        qfResp.PayURL,
	})
}

// PublicCheckUnionPayStatus polls for UnionPay payment completion.
// POST /api/v1/public/:subdomain/checkout/unionpay/check-status
func PublicCheckUnionPayStatus(c *fiber.Ctx) error {
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
		OrderID string `json:"order_id"`
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

	if order.Status == "completed" {
		return c.JSON(fiber.Map{"ok": true, "status": "completed", "order_id": order.ID.String()})
	}

	// Get QFPay order ID
	qfOrderID := ""
	pmID := ""
	if ef := getOrderExtraMap(&order); ef != nil {
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

	var extraFieldsMap map[string]interface{}
	if pm != nil {
		extraFieldsMap = map[string]interface{}(pm.ExtraFields)
	}
	creds := resolveQFPayCredentials("", extraFieldsMap)
	if creds == nil {
		return c.Status(500).JSON(fiber.Map{"error": "QFPay not configured"})
	}

	qfResp, err := qfpayQueryOrder(creds, qfOrderID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("QFPay query failed: %v", err)})
	}

	for _, d := range qfResp.Data {
		if d.Status == "1" {
			if err := completeQFPayOrder(tenant.ID, &order, pm, qfOrderID); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "failed to complete order"})
			}
			return c.JSON(fiber.Map{"ok": true, "status": "completed", "order_id": order.ID.String()})
		}
	}

	return c.JSON(fiber.Map{"ok": true, "status": "pending", "order_id": order.ID.String()})
}

// ──────────────────────────────────────────────────────────────
// Payment Link — UnionPay
// ──────────────────────────────────────────────────────────────

// PaymentLinkCreateUnionPayPayment creates a UnionPay payment for a payment link.
// POST /api/v1/pay/:token/unionpay/create-payment
func PaymentLinkCreateUnionPayPayment(c *fiber.Ctx) error {
	token := c.Params("token")
	link, tenant, order, err := loadPaymentLinkByToken(token)
	if err != nil {
		status := 400
		if link == nil {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	// Find UnionPay payment method
	var pm models.PaymentMethod
	if err := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code = ?",
		tenant.ID, "active", true, GatewayUnionPay).First(&pm).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "UnionPay payment method not configured"})
	}

	extraFields := map[string]interface{}(pm.ExtraFields)
	creds := resolveQFPayCredentials(GatewayUnionPay, extraFields)
	if creds == nil {
		return c.Status(500).JSON(fiber.Map{"error": "UnionPay (QFPay) credentials not configured"})
	}

	currency := strings.ToLower(parseExtraString(pm.ExtraFields, "currency"))
	if currency == "" {
		currency = "hkd"
	}

	amountCents := int64(order.TotalAmount * 100)
	if amountCents <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount"})
	}

	cfg := mustAppConfig()
	notifyURL := cfg.QFPay.NotifyURL
	if notifyURL == "" {
		scheme := c.Protocol()
		host := c.Hostname()
		notifyURL = scheme + "://" + host + "/api/v1/webhooks/qfpay/notify"
	}

	goodsName := fmt.Sprintf("Order %s", order.OrderNumber)
	qfResp, err := qfpayCreatePayment(creds, qfpayUnionPayPayType, amountCents, strings.ToUpper(currency), order.ID.String(), notifyURL, goodsName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("UnionPay payment creation failed: %v", err)})
	}

	// Store payment info
	ef := getOrderExtraMap(order)
	ef["payment_provider"] = "qfpay"
	ef["payment_gateway"] = GatewayUnionPay
	ef["payment_method_id"] = pm.ID.String()
	ef["qfpay_order_id"] = qfResp.OrderID
	ef["qfpay_pay_type"] = qfpayUnionPayPayType
	ef["payment_link_token"] = token
	ef["currency"] = strings.ToUpper(currency)
	database.DB.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).
		Update("extra_fields", models.JSONB(ef))

	return c.JSON(fiber.Map{
		"order_id":       order.ID.String(),
		"order_number":   order.OrderNumber,
		"amount":         order.TotalAmount,
		"currency":       strings.ToUpper(currency),
		"gateway":        GatewayUnionPay,
		"qfpay_order_id": qfResp.OrderID,
		"qr_code":        qfResp.QRCode,
		"pay_url":        qfResp.PayURL,
	})
}

// PaymentLinkCheckUnionPayStatus polls for UnionPay payment link status.
// POST /api/v1/pay/:token/unionpay/check-status
func PaymentLinkCheckUnionPayStatus(c *fiber.Ctx) error {
	token := c.Params("token")
	link, tenant, order, err := loadPaymentLinkByToken(token)
	if err != nil {
		status := 400
		if link == nil {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	if order.Status == "completed" {
		return c.JSON(fiber.Map{"ok": true, "status": "completed", "order_id": order.ID.String()})
	}

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

	var extraFieldsMap map[string]interface{}
	if pm != nil {
		extraFieldsMap = map[string]interface{}(pm.ExtraFields)
	}
	creds := resolveQFPayCredentials("", extraFieldsMap)
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
			database.DB.Model(&models.PaymentLink{}).Where("id = ?", link.ID).Updates(map[string]interface{}{
				"status":     "paid",
				"updated_at": time.Now(),
			})
			return c.JSON(fiber.Map{"ok": true, "status": "completed", "order_id": order.ID.String()})
		}
	}

	return c.JSON(fiber.Map{"ok": true, "status": "pending", "order_id": order.ID.String()})
}
