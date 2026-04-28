package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/paymentintent"

	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/utils"
)

// ===== Public Webstore Checkout (Stripe PaymentIntent + PayPal Buttons) =====
// NOTE:
// - Uses /co/:subdomain/* pages, and requires customer_id cookie (same as RenderPublicPage guard)
// - Uses payment_methods.extra_fields to store gateway credentials

type checkoutItemReq struct {
	ProductID string  `json:"product_id"`
	Quantity  float64 `json:"quantity"`
}

type checkoutCreateReq struct {
	PaymentMethodID    string            `json:"payment_method_id"`
	ShippingMethodID   string            `json:"shipping_method_id"`
	StoreID            string            `json:"store_id"`
	Items              []checkoutItemReq `json:"items"`
	ContactName        string            `json:"contact_name"`
	ContactEmail       string            `json:"contact_email"`
	ContactPhone       string            `json:"contact_phone"`
	ContactAddress     string            `json:"contact_address"`
	DiningTableID      string            `json:"dining_table_id"`
	DiningTableCode    string            `json:"dining_table_code"`
	DiningQueueID      string            `json:"dining_queue_id"`
	DiningTicketNumber string            `json:"dining_ticket_number"`
	// Optional: webstore language at checkout time (e.g. "zh-hant", "zh-hans", "en", "zh-CN", ...)
	Lang string `json:"lang"`
}

func getTenantBySubdomain(subdomain string) (*models.Tenant, error) {
	subdomain = strings.TrimSpace(subdomain)
	if subdomain == "" {
		return nil, errors.New("missing subdomain")
	}

	candidates := []string{subdomain}
	if v, err := url.PathUnescape(subdomain); err == nil && v != subdomain {
		candidates = append(candidates, v)
	}
	if v, err := url.QueryUnescape(subdomain); err == nil && v != subdomain {
		candidates = append(candidates, v)
	}
	if v := url.PathEscape(subdomain); v != subdomain {
		candidates = append(candidates, v)
	}
	if v := url.QueryEscape(subdomain); v != subdomain {
		candidates = append(candidates, v)
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		var tenant models.Tenant
		if err := database.DB.Where("subdomain = ? AND status = ?", candidate, "active").First(&tenant).Error; err == nil {
			return &tenant, nil
		}
	}
	return nil, errors.New("tenant not found")
}

func getCustomerIDFromCookie(c *fiber.Ctx) (*uuid.UUID, error) {
	v := strings.TrimSpace(c.Cookies("customer_id"))
	if v == "" {
		return nil, errors.New("missing customer_id cookie")
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return nil, errors.New("invalid customer_id cookie")
	}
	return &id, nil
}

func parseExtraString(m models.JSONB, key string) string {
	if m == nil {
		return ""
	}
	raw, ok := map[string]interface{}(m)[key]
	if !ok || raw == nil {
		return ""
	}
	switch t := raw.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func loadPaymentMethodByID(tenantID uuid.UUID, idStr string) (*models.PaymentMethod, error) {
	id, err := uuid.Parse(strings.TrimSpace(idStr))
	if err != nil {
		return nil, errors.New("invalid payment method id")
	}
	var pm models.PaymentMethod
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&pm).Error; err != nil {
		return nil, errors.New("payment method not found")
	}
	return &pm, nil
}

func getOrderExtraMap(order *models.Order) map[string]interface{} {
	if order == nil || order.ExtraFields == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}(order.ExtraFields)
}

func createIncomeForPaidOrder(tenantID uuid.UUID, order *models.Order, pm *models.PaymentMethod, provider string, referenceNumber string) error {
	if order == nil || pm == nil {
		return errors.New("missing order or payment method")
	}

	// Avoid duplicates (idempotent)
	var exists int64
	database.DB.Model(&models.Income{}).
		Where("tenant_id = ? AND reference_type = ? AND reference_id = ? AND payment_method = ? AND extra_fields->>'reference_number' = ?",
			tenantID, "order", order.ID, pm.Code, referenceNumber).
		Count(&exists)
	if exists > 0 {
		return nil
	}

	invoiceNumber := ""
	if n, err := reserveNextNumber(tenantID, "invoice_number", "orders"); err == nil {
		invoiceNumber = n
	}

	now := utils.NowInTenantTimezone(tenantID)
	// 與 /orders/new 一致：payment_date 應該反映「實際付款完成日」
	incomeDate := now
	desc := fmt.Sprintf("訂單 %s 的付款", order.OrderNumber)
	if invoiceNumber != "" {
		desc = invoiceNumber
	}

	inc := models.Income{
		TenantID:      tenantID,
		RelatedUserID: nil,
		IncomeType:    "order",
		ReferenceID:   &order.ID,
		ReferenceType: "order",
		Category:      "order",
		Description:   desc,
		Amount:        order.TotalAmount,
		IncomeDate:    incomeDate,
		PaymentMethod: pm.Code,
		BankAccountID: nil,
		Status:        "confirmed",
		Notes:         "",
		CreatedBy:     nil,
		UpdatedBy:     nil,
		CreatedAt:     now,
		UpdatedAt:     now,
		ExtraFields: models.JSONB(map[string]interface{}{
			"payment_method_id": pm.ID.String(),
			"invoice_number":    invoiceNumber,
			"reference_number":  referenceNumber, // stripe pi id / paypal order id
			"order_id":          order.ID.String(),
			"provider":          provider,
		}),
	}
	if err := database.DB.Create(&inc).Error; err != nil {
		return err
	}
	// 同步 Invoice
	syncIncomeToInvoice(database.DB, &inc)
	return nil
}

func buildOrderFromCart(tenantID uuid.UUID, customerID *uuid.UUID, req checkoutCreateReq) (*models.Order, []models.OrderItem, string, error) {
	if len(req.Items) == 0 {
		return nil, nil, "", errors.New("empty cart")
	}
	if customerID == nil || *customerID == uuid.Nil {
		return nil, nil, "", errors.New("missing customer")
	}

	orderNumber, err := utils.GenerateOrderNumber(tenantID)
	if err != nil {
		return nil, nil, "", err
	}

	now := utils.NowInTenantTimezone(tenantID)

	// Load products and compute totals from DB (do NOT trust client price)
	type pricedItem struct {
		Product models.Product
		Qty     float64
	}
	var priced []pricedItem
	for _, it := range req.Items {
		pid, err := uuid.Parse(it.ProductID)
		if err != nil {
			return nil, nil, "", errors.New("invalid product_id")
		}
		if it.Quantity <= 0 {
			return nil, nil, "", errors.New("invalid quantity")
		}
		var p models.Product
		if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ?", pid, tenantID, "active").First(&p).Error; err != nil {
			return nil, nil, "", errors.New("product not found")
		}
		priced = append(priced, pricedItem{Product: p, Qty: it.Quantity})
	}

	var total float64
	var orderItems []models.OrderItem
	for _, it := range priced {
		lineTotal := it.Qty * it.Product.Price
		total += lineTotal
		pid := it.Product.ID
		orderItems = append(orderItems, models.OrderItem{
			TenantID:    tenantID,
			ProductID:   &pid,
			Quantity:    it.Qty,
			UnitPrice:   it.Product.Price,
			TotalPrice:  lineTotal,
			CreatedAt:   now,
			ExtraFields: models.JSONB{},
		})
	}

	// Totals for logistics fee
	totalWeight := 0.0
	totalArea := 0.0
	totalItems := 0
	for _, it := range priced {
		totalItems += int(it.Qty)
		totalWeight += it.Product.Weight * it.Qty
		totalArea += it.Product.Area * it.Qty
	}

	// Resolve shipping method (optional)
	var shippingMethod *models.ShippingMethod
	if strings.TrimSpace(req.ShippingMethodID) != "" {
		smid, err := uuid.Parse(strings.TrimSpace(req.ShippingMethodID))
		if err != nil {
			return nil, nil, "", errors.New("invalid shipping_method_id")
		}
		var sm models.ShippingMethod
		if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ?", smid, tenantID, "active").First(&sm).Error; err != nil {
			return nil, nil, "", errors.New("shipping method not found")
		}
		shippingMethod = &sm
	} else {
		// default shipping method if any
		var sm models.ShippingMethod
		if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").Order("is_default DESC, created_at ASC").First(&sm).Error; err == nil {
			shippingMethod = &sm
		}
	}

	requiresShipping := true
	if shippingMethod != nil {
		requiresShipping = shippingMethod.RequiresShipping
	}

	// If pickup, store_id is required
	var storeID *uuid.UUID
	if !requiresShipping {
		sid := strings.TrimSpace(req.StoreID)
		if sid == "" {
			return nil, nil, "", errors.New("store_id is required for pickup")
		}
		parsed, err := uuid.Parse(sid)
		if err != nil {
			return nil, nil, "", errors.New("invalid store_id")
		}
		storeID = &parsed
	}

	// If delivery, compute best logistics fee based on customer's default address
	bestFee := 0.0
	bestLogisticsID := ""
	if requiresShipping {
		var addr models.CustomerAddress
		if err := database.DB.
			Where("tenant_id = ? AND customer_id = ?", tenantID, *customerID).
			Order("is_default DESC, created_at DESC").
			First(&addr).Error; err != nil {
			return nil, nil, "", errors.New("missing customer default address")
		}

		var companies []models.LogisticsCompany
		if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&companies).Error; err != nil {
			return nil, nil, "", errors.New("failed to load logistics companies")
		}

		type feeRow struct {
			id   string
			name string
			fee  float64
		}
		var best *feeRow
		for _, lc := range companies {
			if !logisticsCompanyMatchesLocation(lc, addr.CountryCode, addr.RegionCode) {
				continue
			}
			f := lc.BaseFee + lc.PerItemFee*float64(totalItems) + lc.PerWeightFee*totalWeight + lc.PerAreaFee*totalArea
			row := feeRow{id: lc.ID.String(), name: lc.Name, fee: f}
			if best == nil || row.fee < best.fee {
				best = &row
			}
		}
		if best == nil {
			return nil, nil, "", errors.New("no logistics company matches customer address")
		}
		bestFee = best.fee
		bestLogisticsID = best.id
	}

	order := &models.Order{
		TenantID:       tenantID,
		OrderNumber:    orderNumber,
		CustomerID:     customerID,
		OrderDate:      now,
		Status:         "pending",
		TotalAmount:    total + bestFee,
		ContactName:    req.ContactName,
		ContactEmail:   req.ContactEmail,
		ContactPhone:   req.ContactPhone,
		ContactAddress: req.ContactAddress,
		CreatedAt:      now,
		UpdatedAt:      now,
		SourceType:     "webstore",
		ExtraFields:    models.JSONB{},
	}

	if shippingMethod != nil {
		order.ShippingMethodID = &shippingMethod.ID
	}
	if storeID != nil {
		order.StoreID = storeID
	}
	// Store shipping fee details into extra_fields (align with orders/new)
	if requiresShipping {
		ef := map[string]interface{}(order.ExtraFields)
		ef["shipping_records"] = []interface{}{
			map[string]interface{}{
				"logistics_company_id": bestLogisticsID,
				"shipping_fee":         bestFee,
			},
		}
		// backward compatible
		ef["shipping_notes"] = ef["shipping_records"]
		order.ExtraFields = models.JSONB(ef)
	}

	if req.DiningTableID != "" || req.DiningTableCode != "" || req.DiningQueueID != "" || req.DiningTicketNumber != "" {
		ef := map[string]interface{}(order.ExtraFields)
		if strings.TrimSpace(req.DiningTableID) != "" {
			ef["dining_table_id"] = strings.TrimSpace(req.DiningTableID)
		}
		if strings.TrimSpace(req.DiningTableCode) != "" {
			ef["dining_table_code"] = strings.TrimSpace(req.DiningTableCode)
		}
		if strings.TrimSpace(req.DiningQueueID) != "" {
			ef["dining_queue_id"] = strings.TrimSpace(req.DiningQueueID)
		}
		if strings.TrimSpace(req.DiningTicketNumber) != "" {
			ef["dining_ticket_number"] = strings.TrimSpace(req.DiningTicketNumber)
		}
		order.ExtraFields = models.JSONB(ef)
	}

	return order, orderItems, orderNumber, nil
}

// PublicCreateStripePaymentIntent creates an Order + PaymentIntent and returns client_secret + publishable key.
func PublicCreateStripePaymentIntent(c *fiber.Ctx) error {
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
	if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ? AND is_online_payment = ?", pmID, tenant.ID, "active", true).First(&pm).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "payment method not found"})
	}
	if strings.ToLower(strings.TrimSpace(pm.Code)) != "stripe" && strings.ToLower(strings.TrimSpace(pm.Code)) != "stripe_connect" {
		return c.Status(400).JSON(fiber.Map{"error": "payment method is not stripe"})
	}

	// Stripe Connect mode: use platform key + destination charges
	useConnect := strings.ToLower(strings.TrimSpace(pm.Code)) == "stripe_connect"

	var stripeSecret, stripePublishable string
	if useConnect {
		if !isStripeConnectReady(tenant) {
			return c.Status(400).JSON(fiber.Map{"error": "Stripe Connect 尚未完成設定"})
		}
		cfg := mustAppConfig()
		stripeSecret = strings.TrimSpace(cfg.Stripe.SecretKey)
		stripePublishable = strings.TrimSpace(cfg.Stripe.PublishableKey)
		if stripeSecret == "" || stripePublishable == "" {
			return c.Status(500).JSON(fiber.Map{"error": "platform stripe keys not configured"})
		}
	} else {
		stripeSecret = parseExtraString(pm.ExtraFields, "stripe_secret_key")
		stripePublishable = parseExtraString(pm.ExtraFields, "stripe_api_key")
		if stripeSecret == "" || stripePublishable == "" {
			return c.Status(400).JSON(fiber.Map{"error": "missing stripe keys in payment method"})
		}
	}

	// Build order (DB-priced)
	order, orderItems, _, err := buildOrderFromCart(tenant.ID, customerID, req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Persist customer's webstore language (best-effort)
	if customerID != nil && *customerID != uuid.Nil {
		lang := strings.TrimSpace(req.Lang)
		if lang == "" {
			lang = strings.TrimSpace(c.Query("lang", ""))
		}
		if lang != "" {
			var customer models.Customer
			if err := database.DB.Where("id = ? AND tenant_id = ?", *customerID, tenant.ID).First(&customer).Error; err == nil {
				ef := map[string]interface{}(customer.ExtraFields)
				if ef == nil {
					ef = map[string]interface{}{}
				}
				ef["language"] = lang
				_ = database.DB.Model(&models.Customer{}).
					Where("id = ? AND tenant_id = ?", customer.ID, tenant.ID).
					Update("extra_fields", models.JSONB(ef)).Error
			}
		}
	}

	// Transaction: create order + items first
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

	// Create PaymentIntent
	stripe.Key = stripeSecret
	amountCents := int64(order.TotalAmount * 100)
	if amountCents <= 0 {
		tx.Rollback()
		return c.Status(400).JSON(fiber.Map{"error": "invalid amount"})
	}
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountCents),
		Currency: stripe.String("hkd"),
		Metadata: map[string]string{
			"tenant_id":           tenant.ID.String(),
			"subdomain":           tenant.Subdomain,
			"order_id":            order.ID.String(),
			"order_number":        order.OrderNumber,
			"payment_method_id":   pm.ID.String(),
			"payment_method_code": pm.Code,
		},
	}

	// Stripe Connect: destination charges with application fee
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
		return c.Status(500).JSON(fiber.Map{"error": "failed to create stripe payment intent"})
	}

	// Store payment info into order extra_fields
	ef := map[string]interface{}(order.ExtraFields)
	ef["payment_provider"] = "stripe"
	ef["payment_method_id"] = pm.ID.String()
	ef["stripe_payment_intent_id"] = pi.ID
	order.ExtraFields = models.JSONB(ef)
	if err := tx.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).Update("extra_fields", order.ExtraFields).Error; err != nil {
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
		"currency":        "HKD",
		"publishable_key": stripePublishable,
		"client_secret":   pi.ClientSecret,
		"payment_intent":  pi.ID,
	})
}

type stripeConfirmReq struct {
	OrderID         string `json:"order_id"`
	PaymentIntentID string `json:"payment_intent_id"`
}

// PublicConfirmStripePayment verifies PaymentIntent server-side and marks the order completed.
func PublicConfirmStripePayment(c *fiber.Ctx) error {
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

	// Fetch payment method from order extra_fields (preferred), else fallback to first stripe config
	pmID := ""
	if ef := getOrderExtraMap(&order); ef != nil {
		if v, ok := ef["payment_method_id"].(string); ok {
			pmID = v
		}
	}
	var pm *models.PaymentMethod
	if pmID != "" {
		pm, err = loadPaymentMethodByID(tenant.ID, pmID)
		if err != nil {
			pm = nil
		}
	}
	if pm == nil {
		var p models.PaymentMethod
		if err := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code IN ?", tenant.ID, "active", true, []string{"stripe", "stripe_connect"}).First(&p).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "stripe payment method not configured"})
		}
		pm = &p
	}

	// Determine which Stripe key to use (Connect mode uses platform key)
	isConnect := strings.ToLower(strings.TrimSpace(pm.Code)) == "stripe_connect"
	var confirmStripeSecret string
	if isConnect {
		cfg := mustAppConfig()
		confirmStripeSecret = strings.TrimSpace(cfg.Stripe.SecretKey)
	} else {
		confirmStripeSecret = parseExtraString(pm.ExtraFields, "stripe_secret_key")
	}
	if confirmStripeSecret == "" {
		return c.Status(500).JSON(fiber.Map{"error": "missing stripe secret key"})
	}

	stripe.Key = confirmStripeSecret
	pi, err := paymentintent.Get(piID, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to verify payment intent"})
	}
	if pi.Status != stripe.PaymentIntentStatusSucceeded && pi.Status != stripe.PaymentIntentStatusProcessing {
		return c.Status(400).JSON(fiber.Map{"error": "payment not completed", "status": pi.Status})
	}

	// Mark completed + create income record
	tx := database.DB.Begin()
	if err := tx.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).Updates(map[string]interface{}{
		"status":     "completed",
		"updated_at": utils.NowInTenantTimezone(tenant.ID),
	}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "failed to update order status"})
	}
	if err := createIncomeForPaidOrder(tenant.ID, &order, pm, "stripe", piID); err != nil {
		// don't block success if income fails
	}
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "transaction failed"})
	}

	return c.JSON(fiber.Map{"ok": true, "order_id": order.ID.String(), "status": "completed"})
}

// --- PayPal helpers (Buttons in-page needs server create/capture) ---

func paypalBaseURL() string {
	// Default to live; allow override via env-like config in app config later.
	// For now, use sandbox if PAYPAL_ENV=sandbox is present in ExtraFields on payment method.
	return "https://api-m.paypal.com"
}

func paypalGetAccessToken(clientID, secret, baseURL string) (string, error) {
	form := "grant_type=client_credentials"
	req, _ := http.NewRequest("POST", baseURL+"/v1/oauth2/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + secret))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("paypal token failed: %s", string(body))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return "", errors.New("paypal missing access_token")
	}
	return out.AccessToken, nil
}

type paypalCreateResp struct {
	ID string `json:"id"`
}

type paypalCreateReq struct {
	PaymentMethodID  string            `json:"payment_method_id"`
	ShippingMethodID string            `json:"shipping_method_id"`
	StoreID          string            `json:"store_id"`
	Items            []checkoutItemReq `json:"items"`
	ContactName      string            `json:"contact_name"`
	ContactEmail     string            `json:"contact_email"`
	ContactPhone     string            `json:"contact_phone"`
	ContactAddress   string            `json:"contact_address"`
	// Optional: webstore language at checkout time (e.g. "zh-hant", "zh-hans", "en", "zh-CN", ...)
	Lang string `json:"lang"`
}

func PublicCreatePayPalOrder(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	customerID, err := getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var req paypalCreateReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	pmID, err := uuid.Parse(strings.TrimSpace(req.PaymentMethodID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payment_method_id"})
	}

	var pm models.PaymentMethod
	if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ? AND is_online_payment = ?", pmID, tenant.ID, "active", true).First(&pm).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "payment method not found"})
	}
	if strings.ToLower(strings.TrimSpace(pm.Code)) != "paypal" {
		return c.Status(400).JSON(fiber.Map{"error": "payment method is not paypal"})
	}

	clientID := parseExtraString(pm.ExtraFields, "paypal_client_id")
	secret := parseExtraString(pm.ExtraFields, "paypal_secret")
	env := strings.ToLower(parseExtraString(pm.ExtraFields, "paypal_env")) // optional: "sandbox" or "live"
	if clientID == "" || secret == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing paypal keys in payment method"})
	}
	baseURL := paypalBaseURL()
	if env == "sandbox" {
		baseURL = "https://api-m.sandbox.paypal.com"
	}

	order, orderItems, _, err := buildOrderFromCart(tenant.ID, customerID, checkoutCreateReq{
		PaymentMethodID:  req.PaymentMethodID,
		ShippingMethodID: req.ShippingMethodID,
		StoreID:          req.StoreID,
		Items:            req.Items,
		ContactName:      req.ContactName,
		ContactEmail:     req.ContactEmail,
		ContactPhone:     req.ContactPhone,
		ContactAddress:   req.ContactAddress,
		Lang:             req.Lang,
	})
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Persist customer's webstore language (best-effort)
	if customerID != nil && *customerID != uuid.Nil {
		lang := strings.TrimSpace(req.Lang)
		if lang == "" {
			lang = strings.TrimSpace(c.Query("lang", ""))
		}
		if lang != "" {
			var customer models.Customer
			if err := database.DB.Where("id = ? AND tenant_id = ?", *customerID, tenant.ID).First(&customer).Error; err == nil {
				ef := map[string]interface{}(customer.ExtraFields)
				if ef == nil {
					ef = map[string]interface{}{}
				}
				ef["language"] = lang
				_ = database.DB.Model(&models.Customer{}).
					Where("id = ? AND tenant_id = ?", customer.ID, tenant.ID).
					Update("extra_fields", models.JSONB(ef)).Error
			}
		}
	}

	// Create order + items in DB first
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

	token, err := paypalGetAccessToken(clientID, secret, baseURL)
	if err != nil {
		tx.Rollback()
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
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "paypal create order failed"})
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "paypal create order failed", "detail": string(body)})
	}
	var out paypalCreateResp
	if err := json.Unmarshal(body, &out); err != nil || strings.TrimSpace(out.ID) == "" {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "paypal create order parse failed"})
	}

	ef := map[string]interface{}(order.ExtraFields)
	ef["payment_provider"] = "paypal"
	ef["payment_method_id"] = pm.ID.String()
	ef["paypal_order_id"] = out.ID
	order.ExtraFields = models.JSONB(ef)
	if err := tx.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenant.ID).Update("extra_fields", order.ExtraFields).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "failed to update order payment info"})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "transaction failed"})
	}

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

type paypalCaptureReq struct {
	OrderID       string `json:"order_id"`
	PayPalOrderID string `json:"paypal_order_id"`
}

func PublicCapturePayPalOrder(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	_, err = getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var req paypalCaptureReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	oid, err := uuid.Parse(strings.TrimSpace(req.OrderID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid order_id"})
	}
	ppID := strings.TrimSpace(req.PayPalOrderID)
	if ppID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing paypal_order_id"})
	}

	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ?", oid, tenant.ID).First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "order not found"})
	}

	// Find paypal payment method from order extra_fields first
	pmID := ""
	if ef := getOrderExtraMap(&order); ef != nil {
		if v, ok := ef["payment_method_id"].(string); ok {
			pmID = v
		}
	}
	var pm *models.PaymentMethod
	if pmID != "" {
		pm, err = loadPaymentMethodByID(tenant.ID, pmID)
		if err != nil {
			pm = nil
		}
	}
	if pm == nil {
		var p models.PaymentMethod
		if err := database.DB.Where("tenant_id = ? AND status = ? AND is_online_payment = ? AND code = ?", tenant.ID, "active", true, "paypal").First(&p).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "paypal payment method not configured"})
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

	token, err := paypalGetAccessToken(clientID, secret, baseURL)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "paypal auth failed"})
	}

	httpReq, _ := http.NewRequest("POST", baseURL+"/v2/checkout/orders/"+ppID+"/capture", nil)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "paypal capture failed"})
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.Status(500).JSON(fiber.Map{"error": "paypal capture failed", "detail": string(body)})
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
	if err := createIncomeForPaidOrder(tenant.ID, &order, pm, "paypal", ppID); err != nil {
		// don't block success if income fails
	}
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "transaction failed"})
	}

	return c.JSON(fiber.Map{"ok": true, "order_id": order.ID.String(), "status": "completed"})
}
