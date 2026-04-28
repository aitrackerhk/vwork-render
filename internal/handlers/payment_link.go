package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/paymentintent"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
)

// ===== Payment Link: generate shareable checkout links for existing orders =====
// Public routes: /pay/:token (page), /api/v1/pay/:token/* (payment APIs)
// Auth routes:   /api/v1/orders/:id/payment-link (generate)

// generateToken creates a cryptographically random URL-safe token.
func generatePaymentLinkToken() (string, error) {
	b := make([]byte, 24) // 48 hex chars
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GeneratePaymentLink creates (or returns existing) payment link for an order.
// POST /api/v1/orders/:id/payment-link
func GeneratePaymentLink(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid order id"})
	}

	// Verify order exists and belongs to tenant
	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", orderID, tenantID).First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "order not found"})
	}

	// Check if order is already completed
	if order.Status == "completed" {
		return c.Status(400).JSON(fiber.Map{"error": "order is already completed"})
	}

	// Parse optional fields from body
	type linkReq struct {
		ExpiresInHours int    `json:"expires_in_hours"` // 0 = no expiry
		Notes          string `json:"notes"`
	}
	var req linkReq
	_ = c.BodyParser(&req) // optional

	// Check if there's an existing active link for this order
	var existing models.PaymentLink
	err = database.DB.Where("order_id = ? AND tenant_id = ? AND status = ?", orderID, tenantID, "active").First(&existing).Error
	if err == nil {
		// Return existing link — build URL from request context
		scheme := c.Protocol()
		host := c.Hostname()
		baseURL := scheme + "://" + host
		return c.JSON(fiber.Map{
			"payment_link": fiber.Map{
				"id":         existing.ID,
				"token":      existing.Token,
				"url":        baseURL + "/pay/" + existing.Token,
				"status":     existing.Status,
				"expires_at": existing.ExpiresAt,
				"created_at": existing.CreatedAt,
			},
		})
	}

	// Generate new token
	token, err := generatePaymentLinkToken()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to generate token"})
	}

	now := utils.NowInTenantTimezone(tenantID)
	link := models.PaymentLink{
		TenantID:  tenantID,
		OrderID:   orderID,
		Token:     token,
		Status:    "active",
		Notes:     strings.TrimSpace(req.Notes),
		CreatedBy: &userID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if req.ExpiresInHours > 0 {
		exp := now.Add(time.Duration(req.ExpiresInHours) * time.Hour)
		link.ExpiresAt = &exp
	}

	if err := database.DB.Create(&link).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create payment link"})
	}

	scheme := c.Protocol()
	host := c.Hostname()
	baseURL := scheme + "://" + host
	return c.JSON(fiber.Map{
		"payment_link": fiber.Map{
			"id":         link.ID,
			"token":      link.Token,
			"url":        baseURL + "/pay/" + link.Token,
			"status":     link.Status,
			"expires_at": link.ExpiresAt,
			"created_at": link.CreatedAt,
		},
	})
}

// RevokePaymentLink cancels an active payment link.
// DELETE /api/v1/orders/:id/payment-link
func RevokePaymentLink(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid order id"})
	}

	result := database.DB.Model(&models.PaymentLink{}).
		Where("order_id = ? AND tenant_id = ? AND status = ?", orderID, tenantID, "active").
		Updates(map[string]interface{}{
			"status":     "cancelled",
			"updated_at": time.Now(),
		})

	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "no active payment link found"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// ──────────────────────────────────────────────────────────────
// Public payment page & APIs (no auth required)
// ──────────────────────────────────────────────────────────────

// loadPaymentLinkByToken loads and validates a payment link token.
func loadPaymentLinkByToken(token string) (*models.PaymentLink, *models.Tenant, *models.Order, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil, nil, fmt.Errorf("missing token")
	}

	var link models.PaymentLink
	if err := database.DB.Where("token = ?", token).First(&link).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("invalid payment link")
	}

	if link.Status != "active" {
		return &link, nil, nil, fmt.Errorf("payment link is %s", link.Status)
	}

	if link.ExpiresAt != nil && time.Now().After(*link.ExpiresAt) {
		// Mark as expired
		database.DB.Model(&models.PaymentLink{}).Where("id = ?", link.ID).Update("status", "expired")
		link.Status = "expired"
		return &link, nil, nil, fmt.Errorf("payment link has expired")
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ? AND status IN ?", link.TenantID, []string{"active", "trial_expired"}).First(&tenant).Error; err != nil {
		return &link, nil, nil, fmt.Errorf("tenant not found")
	}

	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", link.OrderID, link.TenantID).
		Preload("OrderItems").Preload("OrderItems.Product").
		First(&order).Error; err != nil {
		return &link, &tenant, nil, fmt.Errorf("order not found")
	}

	if order.Status == "completed" {
		database.DB.Model(&models.PaymentLink{}).Where("id = ?", link.ID).Update("status", "paid")
		link.Status = "paid"
		return &link, &tenant, &order, fmt.Errorf("order is already paid")
	}

	return &link, &tenant, &order, nil
}

// RenderPaymentPage renders the public payment page for a payment link.
// GET /pay/:token
func RenderPaymentPage(c *fiber.Ctx) error {
	token := c.Params("token")
	link, tenant, order, err := loadPaymentLinkByToken(token)

	// Even on error, render the page with error state
	if err != nil {
		status := "error"
		if link != nil {
			status = link.Status
		}
		return c.Render("pages/payment_page", fiber.Map{
			"Title":  "付款",
			"Error":  err.Error(),
			"Status": status,
		})
	}

	// Load available online payment methods for this tenant
	var paymentMethods []models.PaymentMethod
	database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND trashed_at IS NULL",
		tenant.ID, "active", true).Find(&paymentMethods)

	// Determine which payment providers are available
	hasStripe := false
	hasPayPal := false
	var stripePublishableKey string
	var paypalClientID string
	var paypalEnv string

	// New payment methods
	hasAlipay := false
	hasWeChatPay := false
	hasApplePay := false
	hasGooglePay := false
	hasFPS := false
	hasPayMe := false
	hasAlipayHK := false
	hasWeChatHK := false
	hasBoCPay := false
	hasOctopus := false
	hasUnionPay := false
	hasPayNow := false
	hasGrabPay := false

	// Collect all available online payment method details for the template
	type pmInfo struct {
		ID   string `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	}
	var availableMethods []pmInfo

	for _, pm := range paymentMethods {
		code := strings.ToLower(strings.TrimSpace(pm.Code))
		availableMethods = append(availableMethods, pmInfo{
			ID:   pm.ID.String(),
			Code: code,
			Name: pm.Name,
		})
		switch code {
		case "stripe":
			hasStripe = true
			stripePublishableKey = parseExtraString(pm.ExtraFields, "stripe_api_key")
		case "stripe_connect":
			if isStripeConnectReady(tenant) {
				hasStripe = true
				cfg := mustAppConfig()
				stripePublishableKey = strings.TrimSpace(cfg.Stripe.PublishableKey)
			}
		case "paypal":
			hasPayPal = true
			paypalClientID = parseExtraString(pm.ExtraFields, "paypal_client_id")
			paypalEnv = strings.ToLower(parseExtraString(pm.ExtraFields, "paypal_env"))
		case GatewayAlipay:
			hasAlipay = true
			// Ensure we have a Stripe publishable key for Alipay
			if stripePublishableKey == "" {
				if k := parseExtraString(pm.ExtraFields, "stripe_api_key"); k != "" {
					stripePublishableKey = k
				}
			}
		case GatewayWeChatPay:
			hasWeChatPay = true
			if stripePublishableKey == "" {
				if k := parseExtraString(pm.ExtraFields, "stripe_api_key"); k != "" {
					stripePublishableKey = k
				}
			}
		case GatewayApplePay:
			hasApplePay = true
			if stripePublishableKey == "" {
				if k := parseExtraString(pm.ExtraFields, "stripe_api_key"); k != "" {
					stripePublishableKey = k
				}
			}
		case GatewayGooglePay:
			hasGooglePay = true
			if stripePublishableKey == "" {
				if k := parseExtraString(pm.ExtraFields, "stripe_api_key"); k != "" {
					stripePublishableKey = k
				}
			}
		case GatewayFPS:
			hasFPS = true
		case GatewayPayMe:
			hasPayMe = true
		case GatewayAlipayHK:
			hasAlipayHK = true
		case GatewayWeChatHK:
			hasWeChatHK = true
		case GatewayBoCPay:
			hasBoCPay = true
		case GatewayOctopus:
			hasOctopus = true
		case GatewayUnionPay:
			hasUnionPay = true
		case GatewayPayNow:
			hasPayNow = true
			if stripePublishableKey == "" {
				if k := parseExtraString(pm.ExtraFields, "stripe_api_key"); k != "" {
					stripePublishableKey = k
				}
			}
		case GatewayGrabPay:
			hasGrabPay = true
			if stripePublishableKey == "" {
				if k := parseExtraString(pm.ExtraFields, "stripe_api_key"); k != "" {
					stripePublishableKey = k
				}
			}
		}
	}

	// For Stripe-based methods that don't have their own key, try to resolve from tenant's primary Stripe PM
	if stripePublishableKey == "" && (hasAlipay || hasWeChatPay || hasApplePay || hasGooglePay || hasPayNow || hasGrabPay) {
		if _, pub, _, err := resolveStripeKeysForGateway(tenant.ID, &paymentMethods[0], tenant); err == nil && pub != "" {
			stripePublishableKey = pub
		}
	}

	// Build order items data for the template
	type itemData struct {
		Name     string  `json:"name"`
		Quantity float64 `json:"quantity"`
		Price    float64 `json:"price"`
		Total    float64 `json:"total"`
	}
	var items []itemData
	for _, oi := range order.OrderItems {
		name := ""
		if oi.Product != nil {
			name = oi.Product.Name
		}
		if oi.ItemName != nil && *oi.ItemName != "" {
			name = *oi.ItemName
		}
		if name == "" {
			name = "商品"
		}
		items = append(items, itemData{
			Name:     name,
			Quantity: oi.Quantity,
			Price:    oi.UnitPrice,
			Total:    oi.TotalPrice,
		})
	}
	itemsJSON, _ := json.Marshal(items)
	methodsJSON, _ := json.Marshal(availableMethods)

	return c.Render("pages/payment_page", fiber.Map{
		"Title":                "付款 - " + order.OrderNumber,
		"Token":                token,
		"TenantName":           tenant.Name,
		"OrderNumber":          order.OrderNumber,
		"OrderID":              order.ID.String(),
		"TotalAmount":          order.TotalAmount,
		"TotalAmountFormatted": fmt.Sprintf("%.2f", order.TotalAmount),
		"Currency":             "HKD",
		"ContactName":          order.ContactName,
		"ContactEmail":         order.ContactEmail,
		"Items":                string(itemsJSON),
		"HasStripe":            hasStripe,
		"HasPayPal":            hasPayPal,
		"HasAlipay":            hasAlipay,
		"HasWeChatPay":         hasWeChatPay,
		"HasApplePay":          hasApplePay,
		"HasGooglePay":         hasGooglePay,
		"HasFPS":               hasFPS,
		"HasPayMe":             hasPayMe,
		"HasAlipayHK":          hasAlipayHK,
		"HasWeChatHK":          hasWeChatHK,
		"HasBoCPay":            hasBoCPay,
		"HasOctopus":           hasOctopus,
		"HasUnionPay":          hasUnionPay,
		"HasPayNow":            hasPayNow,
		"HasGrabPay":           hasGrabPay,
		"StripePublishableKey": stripePublishableKey,
		"PayPalClientID":       paypalClientID,
		"PayPalEnv":            paypalEnv,
		"Status":               "active",
		"Notes":                link.Notes,
		"AvailableMethods":     string(methodsJSON),
	})
}

// ──────────────────────────────────────────────────────────────
// Payment Link — Stripe endpoints
// ──────────────────────────────────────────────────────────────

// PaymentLinkCreateStripeIntent creates a Stripe PaymentIntent for the linked order.
// POST /api/v1/pay/:token/stripe/payment-intent
func PaymentLinkCreateStripeIntent(c *fiber.Ctx) error {
	token := c.Params("token")
	link, tenant, order, err := loadPaymentLinkByToken(token)
	if err != nil {
		status := 400
		if link == nil {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	// Find stripe payment method
	var pm models.PaymentMethod
	useConnect := false
	err = database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code IN ?",
		tenant.ID, "active", true, []string{"stripe", "stripe_connect"}).
		Order("CASE WHEN code = 'stripe_connect' THEN 0 ELSE 1 END"). // prefer connect
		First(&pm).Error
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "stripe payment method not configured"})
	}

	code := strings.ToLower(strings.TrimSpace(pm.Code))
	useConnect = code == "stripe_connect"

	var stripeSecret, stripePublishable string
	if useConnect {
		if !isStripeConnectReady(tenant) {
			return c.Status(400).JSON(fiber.Map{"error": "Stripe Connect not ready"})
		}
		cfg := mustAppConfig()
		stripeSecret = strings.TrimSpace(cfg.Stripe.SecretKey)
		stripePublishable = strings.TrimSpace(cfg.Stripe.PublishableKey)
	} else {
		stripeSecret = parseExtraString(pm.ExtraFields, "stripe_secret_key")
		stripePublishable = parseExtraString(pm.ExtraFields, "stripe_api_key")
	}
	if stripeSecret == "" || stripePublishable == "" {
		return c.Status(500).JSON(fiber.Map{"error": "stripe keys not configured"})
	}

	// Create PaymentIntent
	stripe.Key = stripeSecret
	amountCents := int64(order.TotalAmount * 100)
	if amountCents <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount"})
	}

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountCents),
		Currency: stripe.String("hkd"),
		Metadata: map[string]string{
			"tenant_id":           tenant.ID.String(),
			"order_id":            order.ID.String(),
			"order_number":        order.OrderNumber,
			"payment_link_token":  token,
			"payment_method_id":   pm.ID.String(),
			"payment_method_code": pm.Code,
		},
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
		return c.Status(500).JSON(fiber.Map{"error": "failed to create payment intent"})
	}

	// Store payment info into order extra_fields
	ef := getOrderExtraMap(order)
	ef["payment_provider"] = "stripe"
	ef["payment_method_id"] = pm.ID.String()
	ef["stripe_payment_intent_id"] = pi.ID
	ef["payment_link_token"] = token
	database.DB.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).
		Update("extra_fields", models.JSONB(ef))

	return c.JSON(fiber.Map{
		"order_id":        order.ID.String(),
		"order_number":    order.OrderNumber,
		"amount":          order.TotalAmount,
		"currency":        "HKD",
		"publishable_key": stripePublishable,
		"client_secret":   pi.ClientSecret,
		"payment_intent":  pi.ID,
	})
}

// PaymentLinkConfirmStripe confirms Stripe payment and marks order completed.
// POST /api/v1/pay/:token/stripe/confirm
func PaymentLinkConfirmStripe(c *fiber.Ctx) error {
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

	// Find stripe payment method
	pmID := ""
	if ef := getOrderExtraMap(order); ef != nil {
		if v, ok := ef["payment_method_id"].(string); ok {
			pmID = v
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

	isConnect := strings.ToLower(strings.TrimSpace(pm.Code)) == "stripe_connect"
	var confirmSecret string
	if isConnect {
		cfg := mustAppConfig()
		confirmSecret = strings.TrimSpace(cfg.Stripe.SecretKey)
	} else {
		confirmSecret = parseExtraString(pm.ExtraFields, "stripe_secret_key")
	}
	if confirmSecret == "" {
		return c.Status(500).JSON(fiber.Map{"error": "missing stripe secret key"})
	}

	stripe.Key = confirmSecret
	pi, err := paymentintent.Get(piID, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to verify payment intent"})
	}
	if pi.Status != stripe.PaymentIntentStatusSucceeded && pi.Status != stripe.PaymentIntentStatusProcessing {
		return c.Status(400).JSON(fiber.Map{"error": "payment not completed", "status": pi.Status})
	}

	// Mark order completed + create income + mark link paid
	tx := database.DB.Begin()
	if err := tx.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).Updates(map[string]interface{}{
		"status":     "completed",
		"updated_at": utils.NowInTenantTimezone(tenant.ID),
	}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "failed to update order"})
	}
	_ = createIncomeForPaidOrder(tenant.ID, order, pm, "stripe", piID)

	tx.Model(&models.PaymentLink{}).Where("id = ?", link.ID).Updates(map[string]interface{}{
		"status":     "paid",
		"updated_at": time.Now(),
	})

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "transaction failed"})
	}

	return c.JSON(fiber.Map{"ok": true, "order_id": order.ID.String(), "status": "completed"})
}

// ──────────────────────────────────────────────────────────────
// Payment Link — PayPal endpoints
// ──────────────────────────────────────────────────────────────

// PaymentLinkCreatePayPalOrder creates a PayPal order for the linked order.
// POST /api/v1/pay/:token/paypal/create-order
func PaymentLinkCreatePayPalOrder(c *fiber.Ctx) error {
	token := c.Params("token")
	link, tenant, order, err := loadPaymentLinkByToken(token)
	if err != nil {
		status := 400
		if link == nil {
			status = 404
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	// Find PayPal payment method
	var pm models.PaymentMethod
	if err := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code = ?",
		tenant.ID, "active", true, "paypal").First(&pm).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "paypal payment method not configured"})
	}

	clientID := parseExtraString(pm.ExtraFields, "paypal_client_id")
	secret := parseExtraString(pm.ExtraFields, "paypal_secret")
	env := strings.ToLower(parseExtraString(pm.ExtraFields, "paypal_env"))
	if clientID == "" || secret == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing paypal keys"})
	}
	baseURL := paypalBaseURL()
	if env == "sandbox" {
		baseURL = "https://api-m.sandbox.paypal.com"
	}

	ppToken, err := paypalGetAccessToken(clientID, secret, baseURL)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "paypal auth failed"})
	}

	amountStr := fmt.Sprintf("%.2f", order.TotalAmount)
	payload := map[string]interface{}{
		"intent": "CAPTURE",
		"purchase_units": []map[string]interface{}{
			{
				"custom_id": order.ID.String(),
				"amount": map[string]string{
					"currency_code": "HKD",
					"value":         amountStr,
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	httpReq, _ := http.NewRequest("POST", baseURL+"/v2/checkout/orders", bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+ppToken)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "paypal create order failed"})
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.Status(500).JSON(fiber.Map{"error": "paypal create order failed", "detail": string(body)})
	}
	var out paypalCreateResp
	if err := json.Unmarshal(body, &out); err != nil || strings.TrimSpace(out.ID) == "" {
		return c.Status(500).JSON(fiber.Map{"error": "paypal create order parse failed"})
	}

	// Store payment info
	ef := getOrderExtraMap(order)
	ef["payment_provider"] = "paypal"
	ef["payment_method_id"] = pm.ID.String()
	ef["paypal_order_id"] = out.ID
	ef["payment_link_token"] = token
	database.DB.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).
		Update("extra_fields", models.JSONB(ef))

	return c.JSON(fiber.Map{
		"order_id":        order.ID.String(),
		"order_number":    order.OrderNumber,
		"paypal_order_id": out.ID,
		"client_id":       clientID,
		"currency":        "HKD",
		"amount":          order.TotalAmount,
		"env":             env,
	})
}

// PaymentLinkCapturePayPal captures the PayPal order and marks it completed.
// POST /api/v1/pay/:token/paypal/capture-order
func PaymentLinkCapturePayPal(c *fiber.Ctx) error {
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
		PayPalOrderID string `json:"paypal_order_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	ppID := strings.TrimSpace(req.PayPalOrderID)
	if ppID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing paypal_order_id"})
	}

	// Find PayPal payment method
	pmID := ""
	if ef := getOrderExtraMap(order); ef != nil {
		if v, ok := ef["payment_method_id"].(string); ok {
			pmID = v
		}
	}
	var pm *models.PaymentMethod
	if pmID != "" {
		pm, _ = loadPaymentMethodByID(tenant.ID, pmID)
	}
	if pm == nil {
		var p models.PaymentMethod
		if err := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code = ?",
			tenant.ID, "active", true, "paypal").First(&p).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "paypal not configured"})
		}
		pm = &p
	}

	clientID := parseExtraString(pm.ExtraFields, "paypal_client_id")
	secret := parseExtraString(pm.ExtraFields, "paypal_secret")
	env := strings.ToLower(parseExtraString(pm.ExtraFields, "paypal_env"))
	if clientID == "" || secret == "" {
		return c.Status(500).JSON(fiber.Map{"error": "missing paypal keys"})
	}
	baseURL := paypalBaseURL()
	if env == "sandbox" {
		baseURL = "https://api-m.sandbox.paypal.com"
	}

	ppToken, err := paypalGetAccessToken(clientID, secret, baseURL)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "paypal auth failed"})
	}

	httpReq, _ := http.NewRequest("POST", baseURL+"/v2/checkout/orders/"+ppID+"/capture", nil)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+ppToken)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "paypal capture failed"})
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.Status(500).JSON(fiber.Map{"error": "paypal capture failed", "detail": string(body)})
	}

	// Mark order completed + create income + mark link paid
	tx := database.DB.Begin()
	if err := tx.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).Updates(map[string]interface{}{
		"status":     "completed",
		"updated_at": utils.NowInTenantTimezone(tenant.ID),
	}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "failed to update order"})
	}
	_ = createIncomeForPaidOrder(tenant.ID, order, pm, "paypal", ppID)

	tx.Model(&models.PaymentLink{}).Where("id = ?", link.ID).Updates(map[string]interface{}{
		"status":     "paid",
		"updated_at": time.Now(),
	})

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "transaction failed"})
	}

	return c.JSON(fiber.Map{"ok": true, "order_id": order.ID.String(), "status": "completed"})
}
