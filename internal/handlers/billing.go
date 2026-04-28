package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/checkout/session"
	"github.com/stripe/stripe-go/v80/customer"
	"github.com/stripe/stripe-go/v80/invoice"
	"github.com/stripe/stripe-go/v80/price"
	"github.com/stripe/stripe-go/v80/subscription"
	stripeWebhook "github.com/stripe/stripe-go/v80/webhook"
)

// GetSubscription 获取订阅信息
func GetSubscription(c *fiber.Ctx) error {
	// 获取当前用户的 tenant_id
	tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "未授权",
		})
	}

	// 获取租户信息
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "租户不存在",
		})
	}

	// 查询订阅信息
	var subscription models.Subscription
	err := database.DB.
		Preload("Plan").
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		First(&subscription).Error

	// Build trial usage info for trial/free tenants
	var trialUsage fiber.Map
	if (tenant.Plan == "trial" || tenant.Plan == "free") &&
		(tenant.SubscriptionID == nil || strings.TrimSpace(*tenant.SubscriptionID) == "") {
		var orderCount int64
		database.DB.Model(&models.Order{}).Where("tenant_id = ?", tenantID).Count(&orderCount)
		trialUsage = fiber.Map{
			"order_count": orderCount,
			"order_limit": 5,
		}
	}

	response := fiber.Map{
		"tenant": fiber.Map{
			"id":               tenant.ID,
			"subdomain":        tenant.Subdomain,
			"plan":             tenant.Plan,
			"trial_expires_at": tenant.TrialExpiresAt,
		},
		"trial_usage": trialUsage,
	}

	if spApp, ok := resolveSalesPartnerApp(&tenant); ok {
		response["sales_partner"] = fiber.Map{
			"code":          spApp.Code,
			"name":          spApp.Name,
			"company":       spApp.Company,
			"monthly_price": spApp.MonthlyPrice,
			"yearly_price":  spApp.YearlyPrice,
			"currency":      spApp.Currency,
			"status":        spApp.Status,
		}
	} else {
		response["sales_partner"] = nil
	}

	// 如果有订阅记录，添加订阅信息
	if err == nil {
		response["subscription"] = fiber.Map{
			"id":                     subscription.ID,
			"plan_id":                subscription.PlanID,
			"plan_name":              subscription.Plan.Name,
			"plan_display_name":      subscription.Plan.DisplayName,
			"stripe_subscription_id": subscription.StripeSubscriptionID,
			"status":                 subscription.Status,
			"current_period_start":   subscription.CurrentPeriodStart,
			"current_period_end":     subscription.CurrentPeriodEnd,
			"cancel_at_period_end":   subscription.CancelAtPeriodEnd,
		}
	}

	return c.JSON(response)
}

func stripeKey() (string, error) {
	cfg := mustAppConfig()
	if strings.TrimSpace(cfg.Stripe.SecretKey) == "" {
		return "", errors.New("missing STRIPE_SECRET_KEY")
	}
	return cfg.Stripe.SecretKey, nil
}

func tenantHostURL(subdomain string, pathAndQuery string) string {
	cfg := mustAppConfig()
	scheme := strings.TrimSpace(cfg.Domain.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	host := strings.TrimSpace(cfg.Domain.BaseDomain)
	if strings.TrimSpace(subdomain) != "" {
		host = strings.TrimSpace(subdomain) + "." + host
	}
	p := pathAndQuery
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return scheme + "://" + host + p
}

// buildWebsiteAddonLineItem creates a one-time line item for the Tailor Made Website service
// and a corresponding HardwarePurchase record. Returns the Stripe line item, purchase ID, and error.
func buildWebsiteAddonLineItem(tenantID uuid.UUID, c *fiber.Ctx) (*stripe.CheckoutSessionLineItemParams, string, error) {
	// Load catalog to get price
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil, "", fmt.Errorf("tenant not found")
	}

	catalog := loadHardwareCatalog(tenant)
	var websiteItem *hardwareCatalogItem
	for _, it := range catalog {
		if it.ID == "tailor_made_website" {
			copy := it
			websiteItem = &copy
			break
		}
	}
	if websiteItem == nil {
		return nil, "", fmt.Errorf("tailor_made_website not found in catalog")
	}

	unitAmountCents := int64(websiteItem.UnitAmount)
	if unitAmountCents <= 0 {
		return nil, "", fmt.Errorf("tailor_made_website has no price set")
	}

	currency := strings.ToLower(strings.TrimSpace(websiteItem.Currency))
	if currency == "" {
		currency = "hkd"
	}

	// Get localized name
	userLang := c.Get("Accept-Language", "zh-TW")
	if strings.Contains(userLang, "zh-CN") || strings.Contains(userLang, "zh-Hans") {
		userLang = "zh-CN"
	} else if strings.Contains(userLang, "en") {
		userLang = "en"
	} else {
		userLang = "zh-TW"
	}
	displayName := getLocalizedName("tailor_made_website", userLang)
	displayDesc := getLocalizedDesc("tailor_made_website", userLang)

	// Create purchase record
	var userID uuid.UUID
	if uid, ok := c.Locals("user_id").(uuid.UUID); ok {
		userID = uid
	}

	purchase := models.HardwarePurchase{
		TenantID:    tenantID,
		Status:      "created",
		Currency:    strings.ToUpper(currency),
		AmountTotal: float64(unitAmountCents) / 100.0,
		Items: models.JSONB{"items": []hardwarePurchaseItem{{
			ID:         "tailor_made_website",
			Name:       displayName,
			Group:      "service",
			Quantity:   1,
			UnitAmount: websiteItem.UnitAmount,
			Currency:   websiteItem.Currency,
		}}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if userID != uuid.Nil {
		purchase.UserID = &userID
	}
	if err := database.DB.Create(&purchase).Error; err != nil {
		return nil, "", fmt.Errorf("failed to create website purchase record")
	}

	lineItem := &stripe.CheckoutSessionLineItemParams{
		PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
			Currency:   stripe.String(currency),
			UnitAmount: stripe.Int64(unitAmountCents),
			ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
				Name:        stripe.String(displayName),
				Description: stripe.String(displayDesc),
			},
		},
		Quantity: stripe.Int64(1),
	}

	return lineItem, purchase.ID.String(), nil
}

// CreateCheckoutSession 创建支付会话
func CreateCheckoutSession(c *fiber.Ctx) error {
	type Request struct {
		Plan       string `json:"plan"`        // vsuite_monthly, vsuite_yearly, vsuite_pro_monthly, vsuite_pro_yearly
		AddWebsite bool   `json:"add_website"` // optional: bundle tailor-made website one-time purchase
	}

	var req Request
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "无效的请求",
		})
	}

	// 获取当前用户的 tenant_id
	tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "未授权",
		})
	}

	planName := strings.ToLower(strings.TrimSpace(req.Plan))
	// Support legacy plan names
	planName = normalizePlanName(planName)
	if !isValidPaidPlan(planName) {
		return c.Status(400).JSON(fiber.Map{"error": "invalid plan name"})
	}

	// Resolve optional website add-on line item + purchase record
	var websiteLineItem *stripe.CheckoutSessionLineItemParams
	var websitePurchaseID string
	if req.AddWebsite {
		li, purchaseID, err := buildWebsiteAddonLineItem(tenantID, c)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "website add-on error: " + err.Error()})
		}
		websiteLineItem = li
		websitePurchaseID = purchaseID
	}

	// Load tenant + plan
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "租户不存在"})
	}

	// 如果是銷售商代碼租戶，走進階版價格邏輯
	if spApp, ok := resolveSalesPartnerApp(&tenant); ok {
		priceID, currency, err := ensureSalesPartnerStripePrice(spApp, planName)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		key, err := stripeKey()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		stripe.Key = key

		// Ensure Stripe customer
		var stripeCustomerID string
		if tenant.StripeCustomerID != nil {
			stripeCustomerID = strings.TrimSpace(*tenant.StripeCustomerID)
		}
		if stripeCustomerID == "" {
			cfg := mustAppConfig()
			params := &stripe.CustomerParams{
				Name: stripe.String(tenant.Name),
				Metadata: map[string]string{
					"tenant_id":  tenant.ID.String(),
					"subdomain":  tenant.Subdomain,
					"app":        cfg.AppName,
					"created_by": "api",
				},
			}
			cust, err := customer.New(params)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Stripe customer 建立失敗"})
			}
			stripeCustomerID = cust.ID
			_ = database.DB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("stripe_customer_id", stripeCustomerID).Error
		}

		cfg := mustAppConfig()
		successURL := strings.TrimSpace(cfg.Stripe.SuccessURL)
		cancelURL := strings.TrimSpace(cfg.Stripe.CancelURL)
		if successURL == "" {
			successURL = tenantHostURL(tenant.Subdomain, "/billing?checkout=success&session_id={CHECKOUT_SESSION_ID}")
		}
		if cancelURL == "" {
			cancelURL = tenantHostURL(tenant.Subdomain, "/billing?checkout=cancel")
		}

		meta := map[string]string{
			"tenant_id": tenant.ID.String(),
			"plan":      planName,
			"subdomain": tenant.Subdomain,
			"sales_partner_code": func() string {
				if spApp.Code != nil {
					return strings.TrimSpace(*spApp.Code)
				}
				return ""
			}(),
			"currency": currency,
		}
		if websitePurchaseID != "" {
			meta["website_purchase_id"] = websitePurchaseID
		}

		lineItems := []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		}
		if websiteLineItem != nil {
			lineItems = append(lineItems, websiteLineItem)
		}

		params := &stripe.CheckoutSessionParams{
			Mode:               stripe.String(string(stripe.CheckoutSessionModeSubscription)),
			PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
			Customer:           stripe.String(stripeCustomerID),
			SuccessURL:         stripe.String(successURL),
			CancelURL:          stripe.String(cancelURL),
			LineItems:          lineItems,
			SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
				Metadata: meta,
			},
			Metadata:          meta,
			ClientReferenceID: stripe.String(tenant.ID.String()),
		}

		s, err := session.New(params)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Stripe Checkout Session 建立失敗"})
		}

		return c.JSON(fiber.Map{
			"checkout_url": s.URL,
			"plan":         planName,
		})
	}

	var plan models.SubscriptionPlan
	if err := database.DB.Where("name = ? AND is_active = true", planName).First(&plan).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "订阅方案不存在或未启用"})
	}

	priceID := strings.TrimSpace(plan.StripePriceID)
	cfg := mustAppConfig()
	if priceID == "" {
		priceID = getConfigPriceID(planName)
	}
	if priceID == "" {
		return c.Status(500).JSON(fiber.Map{"error": "Stripe price id 未設定（請先設定 subscription_plans.stripe_price_id 或對應的 STRIPE_PRICE_* 環境變數）"})
	}

	key, err := stripeKey()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	stripe.Key = key

	// Ensure Stripe customer
	var stripeCustomerID string
	if tenant.StripeCustomerID != nil {
		stripeCustomerID = strings.TrimSpace(*tenant.StripeCustomerID)
	}
	if stripeCustomerID == "" {
		params := &stripe.CustomerParams{
			Name: stripe.String(tenant.Name),
			Metadata: map[string]string{
				"tenant_id":  tenant.ID.String(),
				"subdomain":  tenant.Subdomain,
				"app":        cfg.AppName,
				"created_by": "api",
			},
		}
		cust, err := customer.New(params)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Stripe customer 建立失敗"})
		}
		stripeCustomerID = cust.ID
		if err := database.DB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("stripe_customer_id", stripeCustomerID).Error; err != nil {
			// 不阻斷，但記錄
			fmt.Printf("⚠️ failed to save stripe_customer_id tenant=%s err=%v\n", tenant.ID, err)
		}
	}

	// Success/cancel URLs
	successURL := strings.TrimSpace(cfg.Stripe.SuccessURL)
	cancelURL := strings.TrimSpace(cfg.Stripe.CancelURL)
	if successURL == "" {
		// include session id for possible client-side checks
		successURL = tenantHostURL(tenant.Subdomain, "/billing?checkout=success&session_id={CHECKOUT_SESSION_ID}")
	}
	if cancelURL == "" {
		cancelURL = tenantHostURL(tenant.Subdomain, "/billing?checkout=cancel")
	}

	// Metadata used by webhook
	meta := map[string]string{
		"tenant_id": tenant.ID.String(),
		"plan":      planName,
		"subdomain": tenant.Subdomain,
	}
	if websitePurchaseID != "" {
		meta["website_purchase_id"] = websitePurchaseID
	}

	lineItems := []*stripe.CheckoutSessionLineItemParams{
		{
			Price:    stripe.String(priceID),
			Quantity: stripe.Int64(1),
		},
	}
	if websiteLineItem != nil {
		lineItems = append(lineItems, websiteLineItem)
	}

	params := &stripe.CheckoutSessionParams{
		Mode:               stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		Customer:           stripe.String(stripeCustomerID),
		SuccessURL:         stripe.String(successURL),
		CancelURL:          stripe.String(cancelURL),
		LineItems:          lineItems,
		// subscription metadata (so customer.subscription.updated has it too)
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: meta,
		},
		Metadata:          meta,
		ClientReferenceID: stripe.String(tenant.ID.String()),
	}

	s, err := session.New(params)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Stripe Checkout Session 建立失敗"})
	}

	return c.JSON(fiber.Map{
		"checkout_url": s.URL,
		"plan":         planName,
	})
}

// SyncCheckoutSession 主動同步 Stripe Checkout Session（避免 webhook 未送達時付款後仍被鎖住）
// GET /api/v1/billing/sync-checkout-session?session_id=cs_test_xxx
func SyncCheckoutSession(c *fiber.Ctx) error {
	// 取得當前租戶 tenant_id
	tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "未授权"})
	}

	sessionID := strings.TrimSpace(c.Query("session_id"))
	if sessionID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing session_id"})
	}

	key, err := stripeKey()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	stripe.Key = key

	cs, err := session.Get(sessionID, nil)
	if err != nil || cs == nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid session_id"})
	}

	// 安全檢查：確保 session metadata tenant_id 與當前租戶一致
	metaTenantID := ""
	if cs.Metadata != nil {
		metaTenantID = strings.TrimSpace(cs.Metadata["tenant_id"])
	}
	if metaTenantID == "" || strings.TrimSpace(metaTenantID) != tenantID.String() {
		return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
	}

	_ = handleCheckoutSessionCompleted(cs)
	return c.JSON(fiber.Map{"ok": true})
}

func resolveSalesPartnerApp(tenant *models.Tenant) (*models.SalesPartnerApplication, bool) {
	if tenant == nil || tenant.ExtraFields == nil {
		return nil, false
	}
	codeAny, ok := tenant.ExtraFields["sales_partner_code"]
	if !ok || codeAny == nil {
		return nil, false
	}
	code, ok := codeAny.(string)
	if !ok {
		return nil, false
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, false
	}
	var app models.SalesPartnerApplication
	if err := database.DB.Where("code = ? AND status = ?", code, "approved").First(&app).Error; err != nil {
		return nil, false
	}
	return &app, true
}

func ensureSalesPartnerStripePrice(app *models.SalesPartnerApplication, plan string) (string, string, error) {
	if app == nil {
		return "", "", errors.New("sales partner not found")
	}
	plan = strings.ToLower(strings.TrimSpace(plan))
	plan = normalizePlanName(plan)

	// Sales partners use vSuite Pro+ plan with dynamic pricing
	billingInterval := planBillingInterval(plan)
	if billingInterval == "" {
		return "", "", errors.New("plan must be a valid paid plan")
	}

	var amount float64
	var priceID *string
	if billingInterval == "month" {
		if app.MonthlyPrice != nil {
			amount = *app.MonthlyPrice
		}
		priceID = app.StripePriceIDMonthly
	} else {
		if app.YearlyPrice != nil {
			amount = *app.YearlyPrice
		}
		priceID = app.StripePriceIDYearly
	}

	if amount <= 0 {
		return "", "", errors.New("sales partner pricing not configured")
	}

	currency := "hkd"
	if app.Currency != nil && strings.TrimSpace(*app.Currency) != "" {
		currency = strings.ToLower(strings.TrimSpace(*app.Currency))
	}

	if priceID != nil && strings.TrimSpace(*priceID) != "" {
		return strings.TrimSpace(*priceID), currency, nil
	}

	key, err := stripeKey()
	if err != nil {
		return "", "", err
	}
	stripe.Key = key

	cfg := mustAppConfig()
	productID := strings.TrimSpace(cfg.Stripe.PartnerProductID)
	if productID == "" {
		return "", "", errors.New("missing STRIPE_PARTNER_PRODUCT_ID")
	}

	unitAmount := int64(math.Round(amount * 100))
	if unitAmount <= 0 {
		return "", "", errors.New("invalid pricing amount")
	}

	interval := billingInterval

	params := &stripe.PriceParams{
		Currency:   stripe.String(currency),
		UnitAmount: stripe.Int64(unitAmount),
		Product:    stripe.String(productID),
		Recurring: &stripe.PriceRecurringParams{
			Interval: stripe.String(interval),
		},
		Metadata: map[string]string{
			"sales_partner_app_id": app.ID.String(),
			"plan":                 plan,
		},
	}

	pr, err := price.New(params)
	if err != nil {
		return "", "", errors.New("failed to create stripe price")
	}

	updateFields := map[string]interface{}{}
	if billingInterval == "month" {
		app.StripePriceIDMonthly = &pr.ID
		updateFields["stripe_price_id_monthly"] = pr.ID
	} else {
		app.StripePriceIDYearly = &pr.ID
		updateFields["stripe_price_id_yearly"] = pr.ID
	}
	if err := database.DB.Model(&models.SalesPartnerApplication{}).Where("id = ?", app.ID).Updates(updateFields).Error; err != nil {
		return "", "", errors.New("failed to save stripe price id")
	}

	return pr.ID, currency, nil
}

// CancelSubscription 取消订阅
func CancelSubscription(c *fiber.Ctx) error {
	// 获取当前用户的 tenant_id
	tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "未授权",
		})
	}

	// 查询订阅信息
	var subRec models.Subscription
	err := database.DB.
		Where("tenant_id = ? AND status = ?", tenantID, "active").
		First(&subRec).Error

	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "未找到活跃的订阅",
		})
	}

	key, err2 := stripeKey()
	if err2 != nil {
		return c.Status(500).JSON(fiber.Map{"error": err2.Error()})
	}
	stripe.Key = key

	// Ask Stripe to cancel at period end
	params := &stripe.SubscriptionParams{CancelAtPeriodEnd: stripe.Bool(true)}
	if _, err := subscription.Update(subRec.StripeSubscriptionID, params); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Stripe 取消订阅失败"})
	}

	subRec.CancelAtPeriodEnd = true
	subRec.UpdatedAt = time.Now()
	if err := database.DB.Save(&subRec).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "取消订阅失败"})
	}

	return c.JSON(fiber.Map{
		"message": "订阅将在当前周期结束时取消",
	})
}

// GetPaymentHistory 获取支付历史
func GetPaymentHistory(c *fiber.Ctx) error {
	// 获取当前用户的 tenant_id
	tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "未授权",
		})
	}

	// 获取租户信息
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "租户不存在",
		})
	}

	// 检查是否有 Stripe Customer ID
	if tenant.StripeCustomerID == nil || strings.TrimSpace(*tenant.StripeCustomerID) == "" {
		return c.JSON(fiber.Map{
			"payments": []fiber.Map{},
			"total":    0,
		})
	}

	key, err := stripeKey()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	stripe.Key = key

	// 从 Stripe 获取发票列表
	params := &stripe.InvoiceListParams{
		Customer: stripe.String(*tenant.StripeCustomerID),
	}
	params.Filters.AddFilter("limit", "", "100") // 限制最多 100 条记录

	invoices := invoice.List(params)
	payments := []fiber.Map{}
	total := 0.0

	for invoices.Next() {
		inv := invoices.Invoice()

		// 只显示已支付的发票
		if inv.Status == stripe.InvoiceStatusPaid || inv.Status == stripe.InvoiceStatusOpen {
			amount := float64(inv.AmountPaid) / 100.0 // 转换为元
			total += amount

			// 格式化日期
			var paidDate *time.Time
			if inv.StatusTransitions.PaidAt > 0 {
				t := time.Unix(inv.StatusTransitions.PaidAt, 0)
				paidDate = &t
			}

			var periodStart, periodEnd *time.Time
			if inv.PeriodStart > 0 {
				t := time.Unix(inv.PeriodStart, 0)
				periodStart = &t
			}
			if inv.PeriodEnd > 0 {
				t := time.Unix(inv.PeriodEnd, 0)
				periodEnd = &t
			}

			payment := fiber.Map{
				"id":                 inv.ID,
				"number":             inv.Number,
				"amount":             amount,
				"currency":           strings.ToUpper(string(inv.Currency)),
				"status":             inv.Status,
				"paid_date":          paidDate,
				"period_start":       periodStart,
				"period_end":         periodEnd,
				"invoice_pdf":        inv.InvoicePDF,
				"hosted_invoice_url": inv.HostedInvoiceURL,
			}

			// 添加订阅信息（如果有）
			if inv.Subscription != nil {
				payment["subscription_id"] = inv.Subscription.ID
			}

			payments = append(payments, payment)
		}
	}

	if err := invoices.Err(); err != nil {
		// 如果获取失败，返回空列表而不是错误
		fmt.Printf("⚠️ failed to list invoices for tenant=%s err=%v\n", tenantID, err)
		return c.JSON(fiber.Map{
			"payments": []fiber.Map{},
			"total":    0,
		})
	}

	return c.JSON(fiber.Map{
		"payments": payments,
		"total":    total,
	})
}

// HandleStripeWebhook 处理 Stripe webhook
func HandleStripeWebhook(c *fiber.Ctx) error {
	cfg := mustAppConfig()
	if strings.TrimSpace(cfg.Stripe.WebhookSecret) == "" {
		return c.Status(500).SendString("missing STRIPE_WEBHOOK_SECRET")
	}

	payload := c.Body()
	sig := c.Get("Stripe-Signature")
	if sig == "" {
		return c.Status(400).SendString("missing Stripe-Signature")
	}

	event, err := stripeWebhook.ConstructEvent(payload, sig, cfg.Stripe.WebhookSecret)
	if err != nil {
		return c.Status(400).SendString("invalid signature")
	}

	key, err2 := stripeKey()
	if err2 != nil {
		return c.Status(500).SendString(err2.Error())
	}
	stripe.Key = key

	switch event.Type {
	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := jsonUnmarshal(event.Data.Raw, &cs); err != nil {
			return c.SendStatus(200)
		}
		_ = handleCheckoutSessionCompleted(&cs)
	case "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := jsonUnmarshal(event.Data.Raw, &sub); err != nil {
			return c.SendStatus(200)
		}
		_ = handleSubscriptionUpsert(&sub)
	case "invoice.payment_succeeded", "invoice.payment_failed":
		// 先不做 payment_history，避免擴太大
	default:
		// ignore
	}

	return c.SendStatus(200)
}

func handleCheckoutSessionCompleted(cs *stripe.CheckoutSession) error {
	if cs != nil && cs.Metadata != nil {
		if cs.Metadata["type"] == "hardware_purchase" {
			return handleHardwarePurchaseCheckoutCompleted(cs)
		}
		if cs.Metadata["type"] == "ai_coins_purchase" {
			return handleAICoinsCheckoutCompleted(cs)
		}
	}

	tenantIDStr := ""
	planName := ""
	subdomain := ""
	if cs.Metadata != nil {
		tenantIDStr = cs.Metadata["tenant_id"]
		planName = normalizePlanName(cs.Metadata["plan"])
		subdomain = cs.Metadata["subdomain"]
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(tenantIDStr))
	if err != nil || tenantID == uuid.Nil {
		return nil
	}

	// Update tenant stripe ids
	updates := map[string]interface{}{}
	if cs.Customer != nil && strings.TrimSpace(cs.Customer.ID) != "" {
		updates["stripe_customer_id"] = cs.Customer.ID
	}
	if cs.Subscription != nil && strings.TrimSpace(cs.Subscription.ID) != "" {
		updates["subscription_id"] = cs.Subscription.ID
	}
	if isValidPaidPlan(planName) {
		updates["plan"] = planName
		updates["status"] = "active"
		// trial_expires_at 可以保留，但一般進入付費後可清空
		updates["trial_expires_at"] = nil
	}
	if len(updates) > 0 {
		_ = database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(updates).Error
	}

	// If this subscription checkout included a bundled website purchase, mark it paid
	if cs.Metadata != nil {
		if wpID := strings.TrimSpace(cs.Metadata["website_purchase_id"]); wpID != "" {
			purchaseID, parseErr := uuid.Parse(wpID)
			if parseErr == nil && purchaseID != uuid.Nil {
				wpUpdates := map[string]interface{}{
					"status":     "paid",
					"updated_at": time.Now(),
				}
				if cs.Customer != nil && strings.TrimSpace(cs.Customer.ID) != "" {
					wpUpdates["stripe_customer_id"] = cs.Customer.ID
				}
				if cs.ID != "" {
					wpUpdates["checkout_session_id"] = cs.ID
				}
				if cs.Currency != "" {
					wpUpdates["currency"] = strings.ToUpper(string(cs.Currency))
				}
				_ = database.DB.Model(&models.HardwarePurchase{}).Where("id = ?", purchaseID).Updates(wpUpdates).Error
			}
		}
	}

	// Create/update subscription record from Stripe Subscription object
	if cs.Subscription == nil || strings.TrimSpace(cs.Subscription.ID) == "" {
		return nil
	}
	stripeSub, err := subscription.Get(cs.Subscription.ID, nil)
	if err != nil {
		return nil
	}
	// ensure metadata exists (fallback)
	if stripeSub.Metadata == nil {
		stripeSub.Metadata = map[string]string{}
	}
	if stripeSub.Metadata["tenant_id"] == "" {
		stripeSub.Metadata["tenant_id"] = tenantID.String()
	}
	if stripeSub.Metadata["plan"] == "" {
		stripeSub.Metadata["plan"] = planName
	}
	if stripeSub.Metadata["subdomain"] == "" {
		stripeSub.Metadata["subdomain"] = subdomain
	}
	return handleSubscriptionUpsert(stripeSub)
}

func handleSubscriptionUpsert(sub *stripe.Subscription) error {
	tenantIDStr := ""
	planName := ""
	if sub.Metadata != nil {
		tenantIDStr = sub.Metadata["tenant_id"]
		planName = normalizePlanName(sub.Metadata["plan"])
	}

	// Find tenant id: metadata first, else by stripe_subscription_id in DB
	var tenantID uuid.UUID
	if tid, err := uuid.Parse(strings.TrimSpace(tenantIDStr)); err == nil {
		tenantID = tid
	}
	if tenantID == uuid.Nil {
		var existing models.Subscription
		if err := database.DB.Where("stripe_subscription_id = ?", sub.ID).First(&existing).Error; err == nil {
			tenantID = existing.TenantID
		} else {
			return nil
		}
	}

	// Determine plan
	if !isValidPaidPlan(planName) {
		// fallback: try to infer from interval (defaults to vsuite tier)
		if sub.Items != nil && len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
			if sub.Items.Data[0].Price.Recurring != nil {
				switch sub.Items.Data[0].Price.Recurring.Interval {
				case "month":
					planName = "vsuite_monthly"
				case "year":
					planName = "vsuite_yearly"
				}
			}
		}
	}

	var plan models.SubscriptionPlan
	if isValidPaidPlan(planName) {
		_ = database.DB.Where("name = ?", planName).First(&plan).Error
	}

	status := string(sub.Status)
	var cps, cpe *time.Time
	if sub.CurrentPeriodStart > 0 {
		t := time.Unix(sub.CurrentPeriodStart, 0)
		cps = &t
	}
	if sub.CurrentPeriodEnd > 0 {
		t := time.Unix(sub.CurrentPeriodEnd, 0)
		cpe = &t
	}

	cancelAtPeriodEnd := sub.CancelAtPeriodEnd

	// Upsert subscription record
	var rec models.Subscription
	err := database.DB.Where("stripe_subscription_id = ?", sub.ID).First(&rec).Error
	if err != nil {
		// create if we have plan id
		if plan.ID == uuid.Nil {
			return nil
		}
		rec = models.Subscription{
			TenantID:             tenantID,
			PlanID:               plan.ID,
			StripeSubscriptionID: sub.ID,
			Status:               status,
			CurrentPeriodStart:   cps,
			CurrentPeriodEnd:     cpe,
			CancelAtPeriodEnd:    cancelAtPeriodEnd,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		_ = database.DB.Create(&rec).Error
	} else {
		updates := map[string]interface{}{
			"status":               status,
			"current_period_start": cps,
			"current_period_end":   cpe,
			"cancel_at_period_end": cancelAtPeriodEnd,
			"updated_at":           time.Now(),
		}
		if plan.ID != uuid.Nil {
			updates["plan_id"] = plan.ID
		}
		_ = database.DB.Model(&models.Subscription{}).Where("id = ?", rec.ID).Updates(updates).Error
	}

	// Sync tenant plan/status
	tenantUpdates := map[string]interface{}{
		"subscription_id": sub.ID,
	}
	if sub.Customer != nil && strings.TrimSpace(sub.Customer.ID) != "" {
		tenantUpdates["stripe_customer_id"] = sub.Customer.ID
	}
	if isValidPaidPlan(planName) {
		tenantUpdates["plan"] = planName
	}
	// status mapping
	switch status {
	case "active", "trialing":
		tenantUpdates["status"] = "active"
	case "canceled", "unpaid", "past_due":
		tenantUpdates["status"] = "suspended"
	}
	_ = database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(tenantUpdates).Error

	// 通知管理員：新訂閱
	if status == "active" || status == "trialing" {
		var tenant models.Tenant
		if err := database.DB.Select("id", "name", "subdomain").Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			go email.EnqueueAdminNotification("new_subscription", map[string]string{
				"tenant_name": tenant.Name,
				"subdomain":   tenant.Subdomain,
				"plan_name":   planName,
				"status":      status,
				"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
			})
		}
	}

	return nil
}

func jsonUnmarshal(raw []byte, v interface{}) error {
	return json.Unmarshal(raw, v)
}

// UpdateSubscriptionFromStripe 从 Stripe 更新订阅信息
func UpdateSubscriptionFromStripe(stripeSubscriptionID string) error {
	// 获取本地订阅记录
	var subRec models.Subscription
	if err := database.DB.Where("stripe_subscription_id = ?", stripeSubscriptionID).First(&subRec).Error; err != nil {
		return err
	}

	// 初始化 Stripe Key
	key, err := stripeKey()
	if err != nil {
		return err
	}
	stripe.Key = key

	// 调用 Stripe API 获取最新订阅信息
	stripeSub, err := subscription.Get(stripeSubscriptionID, nil)
	if err != nil {
		return err
	}

	// 确保 Metadata 包含必要信息，以便 handleSubscriptionUpsert 能正确处理
	if stripeSub.Metadata == nil {
		stripeSub.Metadata = map[string]string{}
	}
	if stripeSub.Metadata["tenant_id"] == "" {
		stripeSub.Metadata["tenant_id"] = subRec.TenantID.String()
	}
	// 如果本地知道 plan，且 stripe metadata 中没有，也可以补上
	if stripeSub.Metadata["plan"] == "" && subRec.PlanID != uuid.Nil {
		var plan models.SubscriptionPlan
		if err := database.DB.First(&plan, subRec.PlanID).Error; err == nil {
			stripeSub.Metadata["plan"] = plan.Name
		}
	}

	// 复用 handleSubscriptionUpsert 逻辑同步状态
	return handleSubscriptionUpsert(stripeSub)
}

// CreateSubscriptionRecord 创建订阅记录
func CreateSubscriptionRecord(tenantID uuid.UUID, planName string, stripeSubID string) error {
	// 查询计划
	var plan models.SubscriptionPlan
	if err := database.DB.Where("name = ?", planName).First(&plan).Error; err != nil {
		return err
	}

	// 创建订阅记录
	now := time.Now()
	var periodEnd time.Time
	if plan.Interval == "month" {
		periodEnd = now.AddDate(0, 1, 0)
	} else {
		periodEnd = now.AddDate(1, 0, 0)
	}

	subscription := models.Subscription{
		TenantID:             tenantID,
		PlanID:               plan.ID,
		StripeSubscriptionID: stripeSubID,
		Status:               "active",
		CurrentPeriodStart:   &now,
		CurrentPeriodEnd:     &periodEnd,
		CancelAtPeriodEnd:    false,
	}

	return database.DB.Create(&subscription).Error
}

// --- Plan name helpers ---

// validPaidPlans is the set of recognized paid plan names
var validPaidPlans = map[string]bool{
	"vsuite_monthly":          true,
	"vsuite_yearly":           true,
	"vsuite_pro_monthly":      true,
	"vsuite_pro_yearly":       true,
	"vsuite_pro_plus_monthly": true,
	"vsuite_pro_plus_yearly":  true,
}

// normalizePlanName maps legacy plan names to the new naming convention.
// "monthly" → "vsuite_monthly", "yearly" → "vsuite_yearly".
// Already-valid names pass through unchanged.
func normalizePlanName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "monthly":
		return "vsuite_monthly"
	case "yearly":
		return "vsuite_yearly"
	}
	return name
}

// isValidPaidPlan returns true if name is one of the four recognized paid plans.
func isValidPaidPlan(name string) bool {
	return validPaidPlans[name]
}

// planBillingInterval returns "month" or "year" for a paid plan name, or "" if unknown.
func planBillingInterval(name string) string {
	switch name {
	case "vsuite_monthly", "vsuite_pro_monthly", "vsuite_pro_plus_monthly":
		return "month"
	case "vsuite_yearly", "vsuite_pro_yearly", "vsuite_pro_plus_yearly":
		return "year"
	}
	return ""
}

// getConfigPriceID returns the Stripe Price ID from config for a given plan name.
func getConfigPriceID(planName string) string {
	appCfg := mustAppConfig()
	switch planName {
	case "vsuite_monthly":
		return strings.TrimSpace(appCfg.Stripe.PriceMonthly)
	case "vsuite_yearly":
		return strings.TrimSpace(appCfg.Stripe.PriceYearly)
	case "vsuite_pro_monthly":
		return strings.TrimSpace(appCfg.Stripe.PriceMonthlyPro)
	case "vsuite_pro_yearly":
		return strings.TrimSpace(appCfg.Stripe.PriceYearlyPro)
	case "vsuite_pro_plus_monthly":
		return strings.TrimSpace(appCfg.Stripe.PriceMonthlyProPlus)
	case "vsuite_pro_plus_yearly":
		return strings.TrimSpace(appCfg.Stripe.PriceYearlyProPlus)
	}
	return ""
}
