package handlers

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/paymentintent"

	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/utils"
)

// ===== Stripe Native Payment Methods =====
// Alipay, WeChat Pay, PayNow, GrabPay — uses Stripe PaymentIntent with specific payment_method_types
// Apple Pay, Google Pay — uses Stripe PaymentIntent; frontend uses Payment Request Button
//
// All these methods share the same backend flow:
//   1. Create PaymentIntent with the correct payment_method_types
//   2. Return client_secret to frontend
//   3. Frontend handles confirmation (redirect for Alipay/WeChat/PayNow/GrabPay, sheet for Apple/Google Pay)
//   4. Confirm endpoint verifies PI status and marks order completed

// resolveStripeKeysForGateway resolves Stripe API keys for a given payment method.
// For Stripe native methods, credentials come from:
//   - The gateway payment method's own ExtraFields (if tenant has dedicated Stripe keys for this method)
//   - Fallback: the tenant's primary "stripe" or "stripe_connect" payment method
//   - Fallback: platform Stripe keys (for Stripe Connect)
func resolveStripeKeysForGateway(tenantID uuid.UUID, pm *models.PaymentMethod, tenant *models.Tenant) (secret, publishable string, useConnect bool, err error) {
	// Check if this payment method has its own Stripe keys
	secret = parseExtraString(pm.ExtraFields, "stripe_secret_key")
	publishable = parseExtraString(pm.ExtraFields, "stripe_api_key")
	if secret != "" && publishable != "" {
		return secret, publishable, false, nil
	}

	// Try to find the tenant's primary Stripe payment method
	var stripePM models.PaymentMethod
	dbErr := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code IN ?",
		tenantID, "active", true, []string{"stripe_connect", "stripe"}).
		Order("CASE WHEN code = 'stripe_connect' THEN 0 ELSE 1 END").
		First(&stripePM).Error

	if dbErr == nil {
		code := strings.ToLower(strings.TrimSpace(stripePM.Code))
		if code == "stripe_connect" && isStripeConnectReady(tenant) {
			cfg := mustAppConfig()
			return strings.TrimSpace(cfg.Stripe.SecretKey), strings.TrimSpace(cfg.Stripe.PublishableKey), true, nil
		}
		s := parseExtraString(stripePM.ExtraFields, "stripe_secret_key")
		p := parseExtraString(stripePM.ExtraFields, "stripe_api_key")
		if s != "" && p != "" {
			return s, p, false, nil
		}
	}

	// Last resort: platform keys (for Stripe Connect mode)
	if isStripeConnectReady(tenant) {
		cfg := mustAppConfig()
		s := strings.TrimSpace(cfg.Stripe.SecretKey)
		p := strings.TrimSpace(cfg.Stripe.PublishableKey)
		if s != "" && p != "" {
			return s, p, true, nil
		}
	}

	return "", "", false, fmt.Errorf("no Stripe keys available for tenant")
}

// stripePaymentMethodTypesForCode returns the Stripe payment_method_types array
// for a given gateway code.
func stripePaymentMethodTypesForCode(code string) []*string {
	switch code {
	case GatewayAlipay:
		return []*string{stripe.String("alipay")}
	case GatewayWeChatPay:
		return []*string{stripe.String("wechat_pay")}
	case GatewayPayNow:
		return []*string{stripe.String("paynow")}
	case GatewayGrabPay:
		return []*string{stripe.String("grabpay")}
	case GatewayApplePay, GatewayGooglePay:
		// Apple Pay and Google Pay go through Stripe's card payment method
		// The frontend Payment Request Button handles wallet detection
		return []*string{stripe.String("card")}
	default:
		return nil
	}
}

// ──────────────────────────────────────────────────────────────
// Webstore Public Checkout — Stripe Native Methods
// ──────────────────────────────────────────────────────────────

// PublicCreateStripeNativePaymentIntent creates an Order + Stripe PaymentIntent
// for Alipay, WeChat Pay, PayNow, GrabPay, Apple Pay, or Google Pay.
// POST /api/v1/public/:subdomain/checkout/stripe-native/payment-intent
func PublicCreateStripeNativePaymentIntent(c *fiber.Ctx) error {
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
	if !IsStripeNativeMethod(code) && !IsStripePaymentRequestMethod(code) {
		return c.Status(400).JSON(fiber.Map{"error": "payment method is not a Stripe native method"})
	}

	pmTypes := stripePaymentMethodTypesForCode(code)
	if pmTypes == nil {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported payment method type"})
	}

	// Resolve Stripe keys
	stripeSecret, stripePublishable, useConnect, err := resolveStripeKeysForGateway(tenant.ID, &pm, tenant)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Determine currency (default HKD, or from payment method ExtraFields)
	currency := strings.ToLower(parseExtraString(pm.ExtraFields, "currency"))
	if currency == "" {
		currency = "hkd"
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

	// Create Stripe PaymentIntent
	stripe.Key = stripeSecret
	amountCents := int64(order.TotalAmount * 100)
	if amountCents <= 0 {
		tx.Rollback()
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount"})
	}

	params := &stripe.PaymentIntentParams{
		Amount:             stripe.Int64(amountCents),
		Currency:           stripe.String(currency),
		PaymentMethodTypes: pmTypes,
		Metadata: map[string]string{
			"tenant_id":           tenant.ID.String(),
			"subdomain":           tenant.Subdomain,
			"order_id":            order.ID.String(),
			"order_number":        order.OrderNumber,
			"payment_method_id":   pm.ID.String(),
			"payment_method_code": pm.Code,
			"gateway":             code,
		},
	}

	// WeChat Pay requires payment_method_options
	if code == GatewayWeChatPay {
		params.PaymentMethodOptions = &stripe.PaymentIntentPaymentMethodOptionsParams{
			WeChatPay: &stripe.PaymentIntentPaymentMethodOptionsWeChatPayParams{
				Client: stripe.String("web"),
			},
		}
	}

	// Stripe Connect: destination charges
	if useConnect {
		params.TransferData = &stripe.PaymentIntentTransferDataParams{
			Destination: tenant.StripeConnectAccountID,
		}
		appFee := connectApplicationFeeAmount(amountCents)
		if appFee > 0 {
			params.ApplicationFeeAmount = stripe.Int64(appFee)
		}
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("failed to create payment intent: %v", err)})
	}

	// Store payment info into order extra_fields
	ef := map[string]interface{}(order.ExtraFields)
	ef["payment_provider"] = "stripe"
	ef["payment_gateway"] = code
	ef["payment_method_id"] = pm.ID.String()
	ef["stripe_payment_intent_id"] = pi.ID
	ef["currency"] = currency
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
		"order_id":        order.ID.String(),
		"order_number":    order.OrderNumber,
		"amount":          order.TotalAmount,
		"currency":        strings.ToUpper(currency),
		"publishable_key": stripePublishable,
		"client_secret":   pi.ClientSecret,
		"payment_intent":  pi.ID,
		"gateway":         code,
	})
}

// PublicConfirmStripeNativePayment verifies Stripe PaymentIntent status for native methods.
// POST /api/v1/public/:subdomain/checkout/stripe-native/confirm
func PublicConfirmStripeNativePayment(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	_, err = getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var req stripeConfirmReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	oid, err := uuid.Parse(strings.TrimSpace(req.OrderID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid order_id"})
	}
	piID := strings.TrimSpace(req.PaymentIntentID)
	if piID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing payment_intent_id"})
	}

	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", oid, tenant.ID).First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "order not found"})
	}

	// Already completed? Idempotent success.
	if order.Status == "completed" {
		return c.JSON(fiber.Map{"ok": true, "order_id": order.ID.String(), "status": "completed"})
	}

	// Find payment method
	pmID := ""
	gatewayCode := ""
	if ef := getOrderExtraMap(&order); ef != nil {
		if v, ok := ef["payment_method_id"].(string); ok {
			pmID = v
		}
		if v, ok := ef["payment_gateway"].(string); ok {
			gatewayCode = v
		}
	}
	var pm *models.PaymentMethod
	if pmID != "" {
		pm, _ = loadPaymentMethodByID(tenant.ID, pmID)
	}
	if pm == nil {
		// Fallback: find any Stripe payment method
		var p models.PaymentMethod
		if err := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code IN ?",
			tenant.ID, "active", true, []string{"stripe", "stripe_connect"}).First(&p).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "stripe payment method not configured"})
		}
		pm = &p
	}

	// Resolve Stripe key for verification
	stripeSecret, _, _, err := resolveStripeKeysForGateway(tenant.ID, pm, tenant)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "missing stripe secret key"})
	}

	stripe.Key = stripeSecret
	pi, err := paymentintent.Get(piID, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to verify payment intent"})
	}
	if pi.Status != stripe.PaymentIntentStatusSucceeded && pi.Status != stripe.PaymentIntentStatusProcessing {
		return c.Status(400).JSON(fiber.Map{"error": "payment not completed", "status": pi.Status})
	}

	// Mark completed + create income
	tx := database.DB.Begin()
	if err := tx.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).Updates(map[string]interface{}{
		"status":     "completed",
		"updated_at": utils.NowInTenantTimezone(tenant.ID),
	}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "failed to update order status"})
	}

	providerLabel := "stripe"
	if gatewayCode != "" {
		providerLabel = gatewayCode
	}
	_ = createIncomeForPaidOrder(tenant.ID, &order, pm, providerLabel, piID)

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "transaction failed"})
	}

	return c.JSON(fiber.Map{"ok": true, "order_id": order.ID.String(), "status": "completed"})
}

// ──────────────────────────────────────────────────────────────
// Payment Link — Stripe Native Methods
// ──────────────────────────────────────────────────────────────

// PaymentLinkCreateStripeNativeIntent creates a Stripe PaymentIntent for native methods on a payment link.
// POST /api/v1/pay/:token/stripe-native/payment-intent
func PaymentLinkCreateStripeNativeIntent(c *fiber.Ctx) error {
	token := c.Params("token")
	link, tenant, order, err := loadPaymentLinkByToken(token)
	if err != nil {
		status := 400
		if link == nil {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	// Parse the requested gateway code from body
	var req struct {
		Gateway string `json:"gateway"` // e.g. "alipay", "wechat_pay", "apple_pay", "google_pay"
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	code := strings.ToLower(strings.TrimSpace(req.Gateway))
	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing gateway code"})
	}
	if !IsStripeNativeMethod(code) && !IsStripePaymentRequestMethod(code) {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported gateway: " + code})
	}

	// Find the payment method for this gateway code
	var pm models.PaymentMethod
	if err := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code = ?",
		tenant.ID, "active", true, code).First(&pm).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": code + " payment method not configured"})
	}

	pmTypes := stripePaymentMethodTypesForCode(code)
	if pmTypes == nil {
		return c.Status(400).JSON(fiber.Map{"error": "unsupported payment method type"})
	}

	stripeSecret, stripePublishable, useConnect, err := resolveStripeKeysForGateway(tenant.ID, &pm, tenant)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Determine currency
	currency := strings.ToLower(parseExtraString(pm.ExtraFields, "currency"))
	if currency == "" {
		currency = "hkd"
	}

	stripe.Key = stripeSecret
	amountCents := int64(order.TotalAmount * 100)
	if amountCents <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount"})
	}

	params := &stripe.PaymentIntentParams{
		Amount:             stripe.Int64(amountCents),
		Currency:           stripe.String(currency),
		PaymentMethodTypes: pmTypes,
		Metadata: map[string]string{
			"tenant_id":           tenant.ID.String(),
			"order_id":            order.ID.String(),
			"order_number":        order.OrderNumber,
			"payment_link_token":  token,
			"payment_method_id":   pm.ID.String(),
			"payment_method_code": pm.Code,
			"gateway":             code,
		},
	}

	if code == GatewayWeChatPay {
		params.PaymentMethodOptions = &stripe.PaymentIntentPaymentMethodOptionsParams{
			WeChatPay: &stripe.PaymentIntentPaymentMethodOptionsWeChatPayParams{
				Client: stripe.String("web"),
			},
		}
	}

	if useConnect {
		params.TransferData = &stripe.PaymentIntentTransferDataParams{
			Destination: tenant.StripeConnectAccountID,
		}
		appFee := connectApplicationFeeAmount(amountCents)
		if appFee > 0 {
			params.ApplicationFeeAmount = stripe.Int64(appFee)
		}
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("failed to create payment intent: %v", err)})
	}

	// Store payment info
	ef := getOrderExtraMap(order)
	ef["payment_provider"] = "stripe"
	ef["payment_gateway"] = code
	ef["payment_method_id"] = pm.ID.String()
	ef["stripe_payment_intent_id"] = pi.ID
	ef["payment_link_token"] = token
	ef["currency"] = currency
	database.DB.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).
		Update("extra_fields", models.JSONB(ef))

	return c.JSON(fiber.Map{
		"order_id":        order.ID.String(),
		"order_number":    order.OrderNumber,
		"amount":          order.TotalAmount,
		"currency":        strings.ToUpper(currency),
		"publishable_key": stripePublishable,
		"client_secret":   pi.ClientSecret,
		"payment_intent":  pi.ID,
		"gateway":         code,
	})
}

// PaymentLinkConfirmStripeNative confirms a Stripe native method payment for a payment link.
// POST /api/v1/pay/:token/stripe-native/confirm
func PaymentLinkConfirmStripeNative(c *fiber.Ctx) error {
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
		PaymentIntentID string `json:"payment_intent_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	piID := strings.TrimSpace(req.PaymentIntentID)
	if piID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing payment_intent_id"})
	}

	// Already completed? Idempotent success.
	if order.Status == "completed" {
		return c.JSON(fiber.Map{"ok": true, "order_id": order.ID.String(), "status": "completed"})
	}

	// Find payment method from order extra_fields
	pmID := ""
	gatewayCode := ""
	if ef := getOrderExtraMap(order); ef != nil {
		if v, ok := ef["payment_method_id"].(string); ok {
			pmID = v
		}
		if v, ok := ef["payment_gateway"].(string); ok {
			gatewayCode = v
		}
	}
	var pm *models.PaymentMethod
	if pmID != "" {
		pm, _ = loadPaymentMethodByID(tenant.ID, pmID)
	}
	if pm == nil {
		var p models.PaymentMethod
		if err := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code IN ?",
			tenant.ID, "active", true, []string{"stripe", "stripe_connect"}).First(&p).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "stripe not configured"})
		}
		pm = &p
	}

	stripeSecret, _, _, err := resolveStripeKeysForGateway(tenant.ID, pm, tenant)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "missing stripe secret key"})
	}

	stripe.Key = stripeSecret
	pi, err := paymentintent.Get(piID, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to verify payment intent"})
	}
	if pi.Status != stripe.PaymentIntentStatusSucceeded && pi.Status != stripe.PaymentIntentStatusProcessing {
		return c.Status(400).JSON(fiber.Map{"error": "payment not completed", "status": pi.Status})
	}

	// Mark completed + income + link paid
	tx := database.DB.Begin()
	if err := tx.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).Updates(map[string]interface{}{
		"status":     "completed",
		"updated_at": utils.NowInTenantTimezone(tenant.ID),
	}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "failed to update order"})
	}

	providerLabel := "stripe"
	if gatewayCode != "" {
		providerLabel = gatewayCode
	}
	_ = createIncomeForPaidOrder(tenant.ID, order, pm, providerLabel, piID)

	tx.Model(&models.PaymentLink{}).Where("id = ?", link.ID).Updates(map[string]interface{}{
		"status":     "paid",
		"updated_at": utils.NowInTenantTimezone(tenant.ID),
	})

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "transaction failed"})
	}

	return c.JSON(fiber.Map{"ok": true, "order_id": order.ID.String(), "status": "completed"})
}

// ──────────────────────────────────────────────────────────────
// Helper: persist customer language
// ──────────────────────────────────────────────────────────────

func persistCustomerLanguage(tenantID uuid.UUID, customerID *uuid.UUID, lang string, queryLang string) {
	if customerID == nil || *customerID == uuid.Nil {
		return
	}
	l := strings.TrimSpace(lang)
	if l == "" {
		l = strings.TrimSpace(queryLang)
	}
	if l == "" {
		return
	}
	var customer models.Customer
	if err := database.DB.Where("id = ? AND tenant_id = ?", *customerID, tenantID).First(&customer).Error; err == nil {
		ef := map[string]interface{}(customer.ExtraFields)
		if ef == nil {
			ef = map[string]interface{}{}
		}
		ef["language"] = l
		_ = database.DB.Model(&models.Customer{}).
			Where("id = ? AND tenant_id = ?", customer.ID, tenantID).
			Update("extra_fields", models.JSONB(ef)).Error
	}
}
