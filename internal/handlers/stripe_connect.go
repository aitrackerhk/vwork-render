package handlers

import (
	"fmt"
	"math"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/account"
	"github.com/stripe/stripe-go/v80/accountlink"
	"github.com/stripe/stripe-go/v80/loginlink"

	"nwork/internal/database"
	"nwork/internal/models"
)

// ========== Stripe Connect：讓租戶一鍵連接 Stripe 收款 ==========
//
// 流程：
//   1. 租戶在設定頁點「連接 Stripe」
//   2. 後端建立 Stripe Express Connected Account
//   3. 生成 Account Link（Stripe 託管的 onboarding 頁）
//   4. 租戶完成 Stripe 註冊/驗證後跳回 vWork
//   5. 之後網店訂單付款透過 Destination Charges，平台自動抽傭金

// GetStripeConnectStatus 取得租戶的 Stripe Connect 狀態
// GET /api/v1/stripe-connect/status
func GetStripeConnectStatus(c *fiber.Ctx) error {
	tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "未授權"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "租戶不存在"})
	}

	result := fiber.Map{
		"connected":           false,
		"onboarding_complete": tenant.StripeConnectOnboardingComplete,
		"account_id":          nil,
		"charges_enabled":     false,
		"payouts_enabled":     false,
		"details_submitted":   false,
		"requirements":        nil,
	}

	if tenant.StripeConnectAccountID == nil || strings.TrimSpace(*tenant.StripeConnectAccountID) == "" {
		return c.JSON(result)
	}

	// Query Stripe for account status
	key, err := stripeKey()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	stripe.Key = key

	acct, err := account.GetByID(*tenant.StripeConnectAccountID, nil)
	if err != nil {
		// Account might have been deleted on Stripe side
		return c.JSON(result)
	}

	result["connected"] = true
	result["account_id"] = acct.ID
	result["charges_enabled"] = acct.ChargesEnabled
	result["payouts_enabled"] = acct.PayoutsEnabled
	result["details_submitted"] = acct.DetailsSubmitted

	// Update onboarding status if changed
	onboardingComplete := acct.ChargesEnabled && acct.DetailsSubmitted
	if onboardingComplete != tenant.StripeConnectOnboardingComplete {
		_ = database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).
			Update("stripe_connect_onboarding_complete", onboardingComplete).Error
	}

	if acct.Requirements != nil {
		result["requirements"] = fiber.Map{
			"currently_due":   acct.Requirements.CurrentlyDue,
			"past_due":        acct.Requirements.PastDue,
			"disabled_reason": acct.Requirements.DisabledReason,
		}
	}

	return c.JSON(result)
}

// CreateStripeConnectAccount 建立 Connected Account 並返回 onboarding 連結
// POST /api/v1/stripe-connect/create-account
func CreateStripeConnectAccount(c *fiber.Ctx) error {
	tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "未授權"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "租戶不存在"})
	}

	// Load current user for prefilling personal info
	var user models.User
	if userID, ok := c.Locals("user_id").(uuid.UUID); ok && userID != uuid.Nil {
		database.DB.Where("id = ?", userID).First(&user)
	}

	key, err := stripeKey()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	stripe.Key = key

	var connectAccountID string

	// If already has an account, reuse it
	if tenant.StripeConnectAccountID != nil && strings.TrimSpace(*tenant.StripeConnectAccountID) != "" {
		connectAccountID = strings.TrimSpace(*tenant.StripeConnectAccountID)
		// Verify it still exists
		if _, err := account.GetByID(connectAccountID, nil); err != nil {
			// Account no longer valid, create a new one
			connectAccountID = ""
		}
	}

	// Create new Express account if needed
	if connectAccountID == "" {
		cfg := mustAppConfig()

		// Determine email: prefer tenant company email, fallback to user email
		email := ""
		if e, ok := tenant.ExtraFields["email"].(string); ok && strings.TrimSpace(e) != "" {
			email = strings.TrimSpace(e)
		} else if strings.TrimSpace(user.Email) != "" {
			email = strings.TrimSpace(user.Email)
		}

		// Determine country code (ISO 3166-1 alpha-2)
		country := ""
		if cc, ok := tenant.ExtraFields["country_code"].(string); ok && strings.TrimSpace(cc) != "" {
			country = strings.ToUpper(strings.TrimSpace(cc))
		}

		// Determine business website URL: prefer custom domain, fallback to subdomain URL
		businessURL := ""
		if customDomain, ok := tenant.ExtraFields["website_custom_domain"].(string); ok && strings.TrimSpace(customDomain) != "" {
			d := strings.TrimSpace(customDomain)
			if !strings.HasPrefix(d, "http") {
				d = "https://" + d
			}
			businessURL = d
		} else if tenant.Subdomain != "" {
			businessURL = tenantHostURL(tenant.Subdomain, "/")
		}

		params := &stripe.AccountParams{
			Type: stripe.String(string(stripe.AccountTypeExpress)),
			BusinessProfile: &stripe.AccountBusinessProfileParams{
				Name: stripe.String(tenant.Name),
			},
			Metadata: map[string]string{
				"tenant_id": tenant.ID.String(),
				"subdomain": tenant.Subdomain,
				"app":       cfg.AppName,
			},
			Capabilities: &stripe.AccountCapabilitiesParams{
				CardPayments: &stripe.AccountCapabilitiesCardPaymentsParams{
					Requested: stripe.Bool(true),
				},
				Transfers: &stripe.AccountCapabilitiesTransfersParams{
					Requested: stripe.Bool(true),
				},
			},
		}

		// Prefill email so Stripe won't ask again
		if email != "" {
			params.Email = stripe.String(email)
		}

		// Prefill country so Stripe won't ask again
		if country != "" {
			params.Country = stripe.String(country)
		}

		// Prefill business URL so Stripe won't ask for a website
		if businessURL != "" {
			params.BusinessProfile.URL = stripe.String(businessURL)
		} else {
			// If no URL, provide product description instead (Stripe requires one of the two)
			params.BusinessProfile.ProductDescription = stripe.String(tenant.Name)
		}

		// Prefill business type
		params.BusinessType = stripe.String("company")

		// Prefill company phone if available
		if phone, ok := tenant.ExtraFields["phone"].(string); ok && strings.TrimSpace(phone) != "" {
			params.Company = &stripe.AccountCompanyParams{
				Phone: stripe.String(strings.TrimSpace(phone)),
			}
		}

		acct, err := account.New(params)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("建立 Stripe Connect 帳號失敗: %v", err)})
		}
		connectAccountID = acct.ID

		// Save to tenant
		if err := database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(map[string]interface{}{
			"stripe_connect_account_id":          connectAccountID,
			"stripe_connect_onboarding_complete": false,
		}).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "儲存 Connect 帳號失敗"})
		}
	}

	// Generate Account Link for onboarding
	cfg := mustAppConfig()
	returnURL := tenantHostURL(tenant.Subdomain, "/settings?tab=payments&stripe_connect=return")
	refreshURL := tenantHostURL(tenant.Subdomain, "/settings?tab=payments&stripe_connect=refresh")

	// For localhost dev, use scheme from config
	if strings.Contains(cfg.Domain.BaseDomain, "localhost") || strings.Contains(cfg.Domain.BaseDomain, "127.0.0.1") {
		returnURL = fmt.Sprintf("%s://%s/settings?tab=payments&stripe_connect=return", cfg.Domain.Scheme, cfg.Domain.BaseDomain)
		refreshURL = fmt.Sprintf("%s://%s/settings?tab=payments&stripe_connect=refresh", cfg.Domain.Scheme, cfg.Domain.BaseDomain)
	}

	linkParams := &stripe.AccountLinkParams{
		Account:    stripe.String(connectAccountID),
		RefreshURL: stripe.String(refreshURL),
		ReturnURL:  stripe.String(returnURL),
		Type:       stripe.String("account_onboarding"),
	}

	link, err := accountlink.New(linkParams)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("生成 onboarding 連結失敗: %v", err)})
	}

	return c.JSON(fiber.Map{
		"account_id":     connectAccountID,
		"onboarding_url": link.URL,
	})
}

// CreateStripeConnectLoginLink 生成 Stripe Express Dashboard 登入連結
// POST /api/v1/stripe-connect/dashboard-link
func CreateStripeConnectLoginLink(c *fiber.Ctx) error {
	tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "未授權"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "租戶不存在"})
	}

	if tenant.StripeConnectAccountID == nil || strings.TrimSpace(*tenant.StripeConnectAccountID) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "尚未連接 Stripe"})
	}

	key, err := stripeKey()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	stripe.Key = key

	link, err := loginlink.New(&stripe.LoginLinkParams{
		Account: tenant.StripeConnectAccountID,
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("生成 Dashboard 連結失敗: %v", err)})
	}

	return c.JSON(fiber.Map{
		"dashboard_url": link.URL,
	})
}

// DisconnectStripeConnect 斷開 Stripe Connect（不刪除 Stripe 帳號，只移除關聯）
// POST /api/v1/stripe-connect/disconnect
func DisconnectStripeConnect(c *fiber.Ctx) error {
	tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "未授權"})
	}

	if err := database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(map[string]interface{}{
		"stripe_connect_account_id":          nil,
		"stripe_connect_onboarding_complete": false,
	}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "斷開失敗"})
	}

	return c.JSON(fiber.Map{"ok": true, "message": "已斷開 Stripe Connect"})
}

// ========== Stripe Connect Checkout Helpers ==========

// isStripeConnectReady 檢查租戶是否已完成 Stripe Connect 設定可收款
func isStripeConnectReady(tenant *models.Tenant) bool {
	return tenant != nil &&
		tenant.StripeConnectAccountID != nil &&
		strings.TrimSpace(*tenant.StripeConnectAccountID) != "" &&
		tenant.StripeConnectOnboardingComplete
}

// connectApplicationFeeAmount 計算平台抽傭金額（單位：cents）
func connectApplicationFeeAmount(totalAmountCents int64) int64 {
	cfg := mustAppConfig()
	pct := cfg.Stripe.ConnectApplicationFeePercent
	if pct <= 0 {
		pct = 2.0 // default 2%
	}
	fee := float64(totalAmountCents) * pct / 100.0
	return int64(math.Round(fee))
}
