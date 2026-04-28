package handlers

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"nwork/internal/database"
	"nwork/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type salesPartnerApplyRequest struct {
	Name        string  `json:"name" form:"name"`
	ContactName string  `json:"contact_name" form:"contact_name"`
	Company     string  `json:"company" form:"company"`
	Email       string  `json:"email" form:"email"`
	Phone       string  `json:"phone" form:"phone"`
	Region      *string `json:"region" form:"region"`
	Message     *string `json:"message" form:"message"`
}

// ApplySalesPartner 公開：銷售商加盟申請
func ApplySalesPartner(c *fiber.Ctx) error {
	var req salesPartnerApplyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	name := strings.TrimSpace(req.Name)
	contactName := strings.TrimSpace(req.ContactName)
	company := strings.TrimSpace(req.Company)
	email := strings.TrimSpace(req.Email)
	phone := strings.TrimSpace(req.Phone)

	if name == "" || contactName == "" || company == "" || email == "" || phone == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name, contact name, company, email and phone are required"})
	}

	var region *string
	if req.Region != nil {
		v := strings.TrimSpace(*req.Region)
		if v != "" {
			region = &v
		}
	}

	var message *string
	if req.Message != nil {
		v := strings.TrimSpace(*req.Message)
		if v != "" {
			message = &v
		}
	}

	app := models.SalesPartnerApplication{
		Name:        name,
		ContactName: contactName,
		Company:     company,
		Email:       email,
		Phone:       phone,
		Region:      region,
		Message:     message,
		Status:      "pending",
	}

	if err := database.DB.Create(&app).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to submit application"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"id":      app.ID,
	})
}

// VWorkAdminSalesPartnerList 取得銷售商加盟申請列表
func VWorkAdminSalesPartnerList(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var apps []models.SalesPartnerApplication
	if err := database.DB.Order("created_at DESC").Find(&apps).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to load applications"})
	}

	return c.JSON(fiber.Map{"data": apps})
}

// VWorkAdminApproveSalesPartner 審核通過並產生 code
func VWorkAdminApproveSalesPartner(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	idParam := strings.TrimSpace(c.Params("id"))
	appID, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var app models.SalesPartnerApplication
	if err := database.DB.Where("id = ?", appID).First(&app).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "application not found"})
	}

	if app.Status == "approved" && app.Code != nil && strings.TrimSpace(*app.Code) != "" {
		return c.JSON(fiber.Map{"success": true, "data": app})
	}

	code, err := generateSalesPartnerCode()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to generate code"})
	}

	if err := ensureUniqueSalesPartnerCode(&code); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to allocate code"})
	}

	now := time.Now()
	app.Status = "approved"
	app.Code = &code
	app.ApprovedAt = &now
	app.RejectedAt = nil

	if err := database.DB.Save(&app).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to approve"})
	}

	return c.JSON(fiber.Map{"success": true, "data": app})
}

// VWorkAdminRejectSalesPartner 拒絕申請
func VWorkAdminRejectSalesPartner(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	idParam := strings.TrimSpace(c.Params("id"))
	appID, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var app models.SalesPartnerApplication
	if err := database.DB.Where("id = ?", appID).First(&app).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "application not found"})
	}

	now := time.Now()
	app.Status = "rejected"
	app.RejectedAt = &now
	app.ApprovedAt = nil

	if err := database.DB.Save(&app).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to reject"})
	}

	return c.JSON(fiber.Map{"success": true, "data": app})
}

type salesPartnerPricingRequest struct {
	MonthlyPrice   *float64 `json:"monthly_price"`
	YearlyPrice    *float64 `json:"yearly_price"`
	Currency       *string  `json:"currency"`
	TrialMonths    *int     `json:"trial_months"`     // 試用月數，最多 2 個月；0 或 null 表示沿用系統預設
	MonthlyAICoins *int     `json:"monthly_ai_coins"` // AI Coins 月配額（2000-2200），null 表示使用預設 2000
}

// VWorkAdminUpdateSalesPartnerPricing 更新銷售商訂閱價格
func VWorkAdminUpdateSalesPartnerPricing(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	appID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var req salesPartnerPricingRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	var app models.SalesPartnerApplication
	if err := database.DB.Where("id = ?", appID).First(&app).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "application not found"})
	}

	if req.MonthlyPrice != nil {
		if *req.MonthlyPrice <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "monthly_price must be > 0"})
		}
		app.MonthlyPrice = req.MonthlyPrice
		// reset cached stripe price when price changes
		app.StripePriceIDMonthly = nil
	}
	if req.YearlyPrice != nil {
		if *req.YearlyPrice <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "yearly_price must be > 0"})
		}
		app.YearlyPrice = req.YearlyPrice
		app.StripePriceIDYearly = nil
	}
	if req.Currency != nil {
		cur := strings.ToLower(strings.TrimSpace(*req.Currency))
		if cur == "" {
			app.Currency = nil
		} else {
			app.Currency = &cur
			app.StripePriceIDMonthly = nil
			app.StripePriceIDYearly = nil
		}
	}

	// 設定試用月數（最多 2 個月）
	if req.TrialMonths != nil {
		months := *req.TrialMonths
		if months < 0 {
			months = 0
		}
		if months > 2 {
			return c.Status(400).JSON(fiber.Map{"error": "trial_months must be <= 2"})
		}
		if months == 0 {
			app.TrialMonths = nil // 0 表示沿用系統預設
		} else {
			app.TrialMonths = &months
		}
	}

	// 設定 AI Coins 月配額（2000-2200）
	if req.MonthlyAICoins != nil {
		coins := *req.MonthlyAICoins
		if coins < models.SalesPartnerMinCoins || coins > models.SalesPartnerMaxCoins {
			return c.Status(400).JSON(fiber.Map{
				"error": fmt.Sprintf("monthly_ai_coins must be between %d and %d", models.SalesPartnerMinCoins, models.SalesPartnerMaxCoins),
			})
		}
		app.MonthlyAICoins = &coins
	}

	if err := database.DB.Save(&app).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update pricing"})
	}

	return c.JSON(fiber.Map{"success": true, "data": app})
}

func generateSalesPartnerCode() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	enc = strings.ToUpper(enc)
	if len(enc) > 10 {
		enc = enc[:10]
	}
	return fmt.Sprintf("SP-%s", enc), nil
}

func ensureUniqueSalesPartnerCode(code *string) error {
	if code == nil || strings.TrimSpace(*code) == "" {
		return fmt.Errorf("empty code")
	}

	for i := 0; i < 6; i++ {
		var existing models.SalesPartnerApplication
		err := database.DB.Where("code = ?", *code).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		newCode, err := generateSalesPartnerCode()
		if err != nil {
			return err
		}
		*code = newCode
	}
	return fmt.Errorf("unable to allocate unique code")
}

// SalesPartnerLandingPage renders the vWork home page with partner-specific pricing.
// Route: GET /:code (catch-all, registered last to avoid conflicts with other routes)
func SalesPartnerLandingPage(c *fiber.Ctx) error {
	code := strings.ToUpper(strings.TrimSpace(c.Params("code")))
	if code == "" {
		return c.Redirect("/")
	}

	var app models.SalesPartnerApplication
	if err := database.DB.Where("UPPER(code) = ? AND status = ?", code, "approved").First(&app).Error; err != nil {
		return c.Redirect("/")
	}

	cfg := mustAppConfig()

	// Build pricing display data
	type PartnerPricing struct {
		Code                string
		Name                string
		Company             string
		MonthlyPriceDisplay string
		YearlyAvgDisplay    string
		YearlyTotalDisplay  string
		YearlySavings       string
		Currency            string
	}

	currency := "HKD"
	if app.Currency != nil && strings.TrimSpace(*app.Currency) != "" {
		currency = strings.ToUpper(strings.TrimSpace(*app.Currency))
	}

	pp := PartnerPricing{
		Code:     code,
		Name:     app.Name,
		Company:  app.Company,
		Currency: currency,
	}

	if app.MonthlyPrice != nil && *app.MonthlyPrice > 0 {
		pp.MonthlyPriceDisplay = formatPrice(*app.MonthlyPrice, currency)
	}
	if app.YearlyPrice != nil && *app.YearlyPrice > 0 {
		yearlyAvg := *app.YearlyPrice / 12
		pp.YearlyAvgDisplay = formatPrice(yearlyAvg, currency)
		pp.YearlyTotalDisplay = formatPrice(*app.YearlyPrice, currency)

		// Calculate savings if both prices are set
		if app.MonthlyPrice != nil && *app.MonthlyPrice > 0 {
			saved := (*app.MonthlyPrice * 12) - *app.YearlyPrice
			if saved > 0 {
				pp.YearlySavings = formatPrice(saved, currency)
			}
		}
	}

	return c.Render("index", fiber.Map{
		"CompanyName":          cfg.CompanyName,
		"TrialDays":            cfg.TrialDays,
		"StripePublishableKey": cfg.Stripe.PublishableKey,
		"WhatsAppPhone":        cfg.WhatsApp.PhoneNumber,
		"SalesPartner":         pp,
	})
}

// formatPrice returns a currency-formatted price string (e.g. "$380", "HK$380")
func formatPrice(amount float64, currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	// Format integer if whole number, otherwise 2 decimals
	var amountStr string
	if amount == float64(int64(amount)) {
		amountStr = fmt.Sprintf("%d", int64(amount))
	} else {
		amountStr = fmt.Sprintf("%.2f", amount)
	}
	// Add thousands separators
	amountStr = addThousandsSep(amountStr)

	switch currency {
	case "HKD":
		return "HK$" + amountStr
	case "USD":
		return "$" + amountStr
	case "TWD":
		return "NT$" + amountStr
	case "CNY", "RMB":
		return "¥" + amountStr
	default:
		return "$" + amountStr
	}
}

// addThousandsSep adds comma separators to a number string
func addThousandsSep(s string) string {
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]
	var result []byte
	for i, c := range intPart {
		pos := len(intPart) - i
		if pos%3 == 0 && i > 0 && c != '-' {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	if len(parts) == 2 {
		return string(result) + "." + parts[1]
	}
	return string(result)
}
