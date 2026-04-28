package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/checkout/session"
	"github.com/stripe/stripe-go/v80/customer"
)

type hardwareCatalogItem struct {
	ID         string  `json:"id"`
	Group      string  `json:"group"`
	UnitAmount float64 `json:"unit_amount,omitempty"`
	Currency   string  `json:"currency,omitempty"`
}

// i18n locale structure for hardware catalog
type i18nHardware struct {
	Catalog     map[string]string `json:"catalog"`
	CatalogDesc map[string]string `json:"catalogDesc"`
}

type i18nLocale struct {
	Hardware i18nHardware `json:"hardware"`
}

var i18nCache = make(map[string]i18nLocale)

func loadI18nLocale(lang string) i18nLocale {
	if cached, ok := i18nCache[lang]; ok {
		return cached
	}

	filename := "zh.json"
	switch lang {
	case "en":
		filename = "en.json"
	case "zh-CN":
		filename = "zh-CN.json"
	}

	path := "web/static/locales/" + filename
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[hardware_purchase] Failed to read i18n file %s: %v", path, err)
		return i18nLocale{}
	}

	var locale i18nLocale
	if err := json.Unmarshal(b, &locale); err != nil {
		log.Printf("[hardware_purchase] Failed to parse i18n JSON: %v", err)
		return i18nLocale{}
	}

	i18nCache[lang] = locale
	return locale
}

func getLocalizedName(itemID, lang string) string {
	locale := loadI18nLocale(lang)
	if name, ok := locale.Hardware.Catalog[itemID]; ok && name != "" {
		return name
	}
	return itemID // fallback to ID
}

func getLocalizedDesc(itemID, lang string) string {
	locale := loadI18nLocale(lang)
	if desc, ok := locale.Hardware.CatalogDesc[itemID]; ok {
		return desc
	}
	return ""
}

type hardwarePurchaseItem struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Group      string  `json:"group"`
	Quantity   int     `json:"quantity"`
	UnitAmount float64 `json:"unit_amount,omitempty"`
	Currency   string  `json:"currency,omitempty"`
}

func defaultHardwareCatalog() []hardwareCatalogItem {
	cfg := mustAppConfig()
	path := strings.TrimSpace(cfg.HardwarePurchaseCatalogFile)
	if path == "" {
		path = "config/hardware_purchase_catalog.json"
	}

	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[hardware_purchase] Failed to read catalog file %s: %v", path, err)
		return []hardwareCatalogItem{}
	}
	log.Printf("[hardware_purchase] Successfully loaded catalog from %s (%d bytes)", path, len(b))

	var items []hardwareCatalogItem
	if err := json.Unmarshal(b, &items); err != nil || len(items) == 0 {
		log.Printf("[hardware_purchase] Failed to parse catalog JSON: %v, items: %d", err, len(items))
		return []hardwareCatalogItem{}
	}
	log.Printf("[hardware_purchase] Parsed %d catalog items", len(items))

	cleaned := make([]hardwareCatalogItem, 0, len(items))
	for _, it := range items {
		it.ID = strings.TrimSpace(it.ID)
		it.Group = strings.TrimSpace(it.Group)
		if it.ID == "" {
			continue
		}
		if it.Group == "" {
			it.Group = "pos"
		}
		cleaned = append(cleaned, it)
	}
	return cleaned
}

func loadHardwareCatalog(tenant models.Tenant) []hardwareCatalogItem {
	// Always use default catalog from JSON file
	// Tenant-specific customization is disabled for now
	return defaultHardwareCatalog()
}

func saveHardwareCatalog(tenantID uuid.UUID, items []hardwareCatalogItem) error {
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return err
	}
	if tenant.ExtraFields == nil {
		tenant.ExtraFields = make(models.JSONB)
	}
	tenant.ExtraFields["hardware_purchase_catalog"] = items
	return database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("extra_fields", tenant.ExtraFields).Error
}

// GetHardwarePurchaseSettings returns catalog settings for current tenant
func GetHardwarePurchaseSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	return c.JSON(fiber.Map{
		"catalog": loadHardwareCatalog(tenant),
	})
}

// UpdateHardwarePurchaseSettings updates catalog settings for current tenant
func UpdateHardwarePurchaseSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		Catalog []hardwareCatalogItem `json:"catalog"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if len(req.Catalog) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "catalog is required"})
	}

	// basic normalize
	cleaned := make([]hardwareCatalogItem, 0, len(req.Catalog))
	for _, it := range req.Catalog {
		it.ID = strings.TrimSpace(it.ID)
		it.Group = strings.TrimSpace(it.Group)
		if it.ID == "" {
			continue
		}
		if it.Group == "" {
			it.Group = "pos"
		}
		cleaned = append(cleaned, it)
	}
	if len(cleaned) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid catalog"})
	}

	if err := saveHardwareCatalog(tenantID, cleaned); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
	}
	return c.JSON(fiber.Map{"success": true})
}

// CreateHardwarePurchaseCheckoutSession creates Stripe Checkout Session for hardware purchase
func CreateHardwarePurchaseCheckoutSession(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}
	userID := middleware.GetUserID(c)

	var req struct {
		Items []struct {
			ID       string `json:"id"`
			Quantity int    `json:"quantity"`
		} `json:"items"`
		ReturnPage string `json:"return_page"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if len(req.Items) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "items is required"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	catalog := loadHardwareCatalog(tenant)
	catalogMap := map[string]hardwareCatalogItem{}
	for _, it := range catalog {
		catalogMap[it.ID] = it
	}

	lineItems := make([]*stripe.CheckoutSessionLineItemParams, 0, len(req.Items))
	purchaseItems := make([]hardwarePurchaseItem, 0, len(req.Items))
	totalAmount := 0.0

	// Get user language preference (from header or default to zh-TW)
	userLang := c.Get("Accept-Language", "zh-TW")
	if strings.Contains(userLang, "zh-CN") || strings.Contains(userLang, "zh-Hans") {
		userLang = "zh-CN"
	} else if strings.Contains(userLang, "en") {
		userLang = "en"
	} else {
		userLang = "zh-TW"
	}

	for _, raw := range req.Items {
		id := strings.TrimSpace(raw.ID)
		if id == "" {
			continue
		}
		qty := raw.Quantity
		if qty <= 0 {
			qty = 1
		}
		item, ok := catalogMap[id]
		if !ok {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("invalid item: %s", id)})
		}

		// Get localized name and description from i18n
		displayName := getLocalizedName(item.ID, userLang)
		displayDesc := getLocalizedDesc(item.ID, userLang)

		// Determine currency
		currency := strings.ToLower(strings.TrimSpace(item.Currency))
		if currency == "" {
			currency = "hkd"
		}

		// Use price_data for dynamic product name (no need to pre-create Stripe products)
		// UnitAmount is in cents (e.g., HKD 100 = 10000 cents)
		unitAmountCents := int64(item.UnitAmount)
		if unitAmountCents <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("price not set for item: %s", displayName)})
		}

		lineItems = append(lineItems, &stripe.CheckoutSessionLineItemParams{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency:   stripe.String(currency),
				UnitAmount: stripe.Int64(unitAmountCents),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String(displayName),
					Description: stripe.String(displayDesc),
					Metadata: map[string]string{
						"catalog_id": item.ID,
						"group":      item.Group,
					},
				},
			},
			Quantity: stripe.Int64(int64(qty)),
		})

		purchaseItems = append(purchaseItems, hardwarePurchaseItem{
			ID:         item.ID,
			Name:       displayName,
			Group:      item.Group,
			Quantity:   qty,
			UnitAmount: item.UnitAmount,
			Currency:   item.Currency,
		})

		if item.UnitAmount > 0 {
			totalAmount += item.UnitAmount * float64(qty)
		}
	}

	if len(lineItems) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "no valid items"})
	}

	key, err := stripeKey()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	stripe.Key = key

	// Ensure Stripe customer (reuse tenant customer id if exists)
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

	// Create purchase record
	purchase := models.HardwarePurchase{
		TenantID:    tenant.ID,
		Status:      "created",
		Currency:    "",
		AmountTotal: totalAmount,
		Items:       models.JSONB{"items": purchaseItems},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if userID != uuid.Nil {
		purchase.UserID = &userID
	}

	if err := database.DB.Create(&purchase).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create purchase"})
	}

	returnPath := "/hardware-purchase"
	switch strings.ToLower(strings.TrimSpace(req.ReturnPage)) {
	case "billing":
		returnPath = "/billing"
	}

	successURL := tenantHostURL(tenant.Subdomain, fmt.Sprintf("%s?checkout=success&session_id={CHECKOUT_SESSION_ID}&purchase_id=%s", returnPath, purchase.ID.String()))
	cancelURL := tenantHostURL(tenant.Subdomain, returnPath+"?checkout=cancel")

	meta := map[string]string{
		"type":        "hardware_purchase",
		"tenant_id":   tenant.ID.String(),
		"user_id":     userID.String(),
		"purchase_id": purchase.ID.String(),
		"subdomain":   tenant.Subdomain,
	}

	params := &stripe.CheckoutSessionParams{
		Mode:               stripe.String(string(stripe.CheckoutSessionModePayment)),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		Customer:           stripe.String(stripeCustomerID),
		SuccessURL:         stripe.String(successURL),
		CancelURL:          stripe.String(cancelURL),
		LineItems:          lineItems,
		Metadata:           meta,
		ClientReferenceID:  stripe.String(purchase.ID.String()),
	}

	s, err := session.New(params)
	if err != nil {
		_ = database.DB.Delete(&purchase).Error
		return c.Status(500).JSON(fiber.Map{"error": "Stripe Checkout Session 建立失敗"})
	}

	updates := map[string]interface{}{
		"checkout_session_id": s.ID,
		"stripe_customer_id":  stripeCustomerID,
		"updated_at":          time.Now(),
	}
	_ = database.DB.Model(&models.HardwarePurchase{}).Where("id = ?", purchase.ID).Updates(updates).Error

	return c.JSON(fiber.Map{
		"checkout_url": s.URL,
		"purchase_id":  purchase.ID,
	})
}

// SyncHardwarePurchaseCheckoutSession syncs payment status by checkout session id
func SyncHardwarePurchaseCheckoutSession(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
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

	if cs.Metadata == nil || cs.Metadata["type"] != "hardware_purchase" {
		return c.Status(400).JSON(fiber.Map{"error": "not hardware purchase session"})
	}

	if err := handleHardwarePurchaseCheckoutCompleted(cs); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "sync failed"})
	}

	return c.JSON(fiber.Map{"success": true})
}

// GetHardwarePurchaseRecords returns purchase records for current tenant
func GetHardwarePurchaseRecords(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var purchases []models.HardwarePurchase
	if err := database.DB.
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(200).
		Find(&purchases).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load purchases"})
	}

	// normalize items
	result := make([]fiber.Map, 0, len(purchases))
	for _, p := range purchases {
		items := []hardwarePurchaseItem{}
		if raw, ok := p.Items["items"]; ok {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &items)
		} else if raw, ok := p.Items["_data"]; ok {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &items)
		}

		result = append(result, fiber.Map{
			"id":                  p.ID,
			"tenant_id":           p.TenantID,
			"user_id":             p.UserID,
			"status":              p.Status,
			"checkout_session_id": p.CheckoutSessionID,
			"payment_intent_id":   p.PaymentIntentID,
			"currency":            p.Currency,
			"amount_total":        p.AmountTotal,
			"items":               items,
			"created_at":          p.CreatedAt,
		})
	}

	return c.JSON(fiber.Map{"data": result})
}

// handleHardwarePurchaseCheckoutCompleted updates purchase status when checkout completes
func handleHardwarePurchaseCheckoutCompleted(cs *stripe.CheckoutSession) error {
	if cs == nil || cs.Metadata == nil {
		return errors.New("invalid session")
	}
	if cs.Metadata["type"] != "hardware_purchase" {
		return nil
	}

	purchaseIDStr := strings.TrimSpace(cs.Metadata["purchase_id"])
	purchaseID, err := uuid.Parse(purchaseIDStr)
	if err != nil || purchaseID == uuid.Nil {
		return errors.New("invalid purchase_id")
	}

	updates := map[string]interface{}{
		"status":     "paid",
		"updated_at": time.Now(),
	}
	if cs.PaymentIntent != nil && strings.TrimSpace(cs.PaymentIntent.ID) != "" {
		pi := cs.PaymentIntent.ID
		updates["payment_intent_id"] = pi
	}
	if cs.Customer != nil && strings.TrimSpace(cs.Customer.ID) != "" {
		updates["stripe_customer_id"] = cs.Customer.ID
	}
	if cs.AmountTotal > 0 {
		updates["amount_total"] = float64(cs.AmountTotal) / 100.0
	}
	if cs.Currency != "" {
		updates["currency"] = strings.ToUpper(string(cs.Currency))
	}
	if cs.ID != "" {
		updates["checkout_session_id"] = cs.ID
	}

	return database.DB.Model(&models.HardwarePurchase{}).Where("id = ?", purchaseID).Updates(updates).Error
}

// SubmitCompanyInfo handles POST /api/v1/hardware-purchase/submit-company-info
// Accepts multipart form with company_link (text) and files (multipart uploads)
// Associates the info with the latest tailor_made_website purchase for the tenant
func SubmitCompanyInfo(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	companyLink := strings.TrimSpace(c.FormValue("company_link"))

	// Handle file uploads
	form, err := c.MultipartForm()
	var fileURLs []string
	if err == nil && form != nil && form.File["files"] != nil {
		for _, fh := range form.File["files"] {
			// Validate file size (max 10MB per file)
			if fh.Size > 10*1024*1024 {
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("File %s exceeds 10MB limit", fh.Filename)})
			}

			// Create upload directory: web/uploads/{tenant_id}/company_info/{date}/
			uploadDir := filepath.Join("web", "uploads", tenantID.String(), "company_info", time.Now().Format("2006-01-02"))
			if mkErr := os.MkdirAll(uploadDir, 0755); mkErr != nil {
				log.Printf("[submit-company-info] Failed to create upload dir: %v", mkErr)
				continue
			}

			// Generate unique filename preserving extension
			ext := filepath.Ext(fh.Filename)
			filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
			destPath := filepath.Join(uploadDir, filename)

			if saveErr := c.SaveFile(fh, destPath); saveErr != nil {
				log.Printf("[submit-company-info] Failed to save file %s: %v", fh.Filename, saveErr)
				continue
			}

			fileURL := fmt.Sprintf("/uploads/%s/company_info/%s/%s", tenantID.String(), time.Now().Format("2006-01-02"), filename)
			fileURLs = append(fileURLs, fileURL)
		}
	}

	if companyLink == "" && len(fileURLs) == 0 {
		return c.JSON(fiber.Map{"success": true, "message": "no data provided, skipped"})
	}

	// Build company_info JSONB
	companyInfo := models.JSONB{
		"company_link": companyLink,
		"files":        fileURLs,
		"submitted_at": time.Now().Format(time.RFC3339),
	}

	// Find the latest tailor_made_website purchase for this tenant (created in the last 30 minutes)
	var purchase models.HardwarePurchase
	cutoff := time.Now().Add(-30 * time.Minute)
	result := database.DB.
		Where("tenant_id = ? AND created_at >= ?", tenantID, cutoff).
		Order("created_at DESC").
		First(&purchase)

	if result.Error != nil {
		// No recent purchase found — store as a standalone record with status "company_info_only"
		log.Printf("[submit-company-info] No recent purchase found for tenant %s, creating standalone record", tenantID)
		standalone := models.HardwarePurchase{
			TenantID:    tenantID,
			Status:      "company_info_only",
			CompanyInfo: companyInfo,
			Items:       models.JSONB{"items": []interface{}{}},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		userID := middleware.GetUserID(c)
		if userID != uuid.Nil {
			standalone.UserID = &userID
		}
		if dbErr := database.DB.Create(&standalone).Error; dbErr != nil {
			log.Printf("[submit-company-info] Failed to create standalone record: %v", dbErr)
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save company info"})
		}
		return c.JSON(fiber.Map{"success": true, "purchase_id": standalone.ID})
	}

	// Update existing purchase with company info
	if dbErr := database.DB.Model(&models.HardwarePurchase{}).
		Where("id = ?", purchase.ID).
		Updates(map[string]interface{}{
			"company_info": companyInfo,
			"updated_at":   time.Now(),
		}).Error; dbErr != nil {
		log.Printf("[submit-company-info] Failed to update purchase %s: %v", purchase.ID, dbErr)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save company info"})
	}

	return c.JSON(fiber.Map{"success": true, "purchase_id": purchase.ID})
}
