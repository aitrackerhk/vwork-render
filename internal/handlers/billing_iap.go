package handlers

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"
)

// ──────────────────────────────────────────────────────────────────
// IAP Config helpers
// ──────────────────────────────────────────────────────────────────

func iapConfig() (cfg struct {
	GoogleServiceAccountJSON string
	GooglePackageName        string
	AppleSharedSecret        string
	AppleBundleID            string
	AppleIssuerID            string
	AppleKeyID               string
	ApplePrivateKey          string
	AppleEnvironment         string // "Production" or "Sandbox"
}) {
	appCfg := mustAppConfig()
	cfg.GoogleServiceAccountJSON = appCfg.IAP.GoogleServiceAccountJSON
	cfg.GooglePackageName = appCfg.IAP.GooglePackageName
	cfg.AppleSharedSecret = appCfg.IAP.AppleSharedSecret
	cfg.AppleBundleID = appCfg.IAP.AppleBundleID
	cfg.AppleIssuerID = appCfg.IAP.AppleIssuerID
	cfg.AppleKeyID = appCfg.IAP.AppleKeyID
	cfg.ApplePrivateKey = appCfg.IAP.ApplePrivateKey
	cfg.AppleEnvironment = appCfg.IAP.AppleEnvironment
	if cfg.AppleEnvironment == "" {
		cfg.AppleEnvironment = "Production"
	}
	return
}

// ──────────────────────────────────────────────────────────────────
// POST /api/v1/billing/iap/verify — Verify IAP purchase (Google or Apple)
// Called by Flutter app after a successful in-app purchase
// ──────────────────────────────────────────────────────────────────

type IAPVerifyRequest struct {
	Platform      string `json:"platform"`       // "google" or "apple"
	ProductID     string `json:"product_id"`     // IAP product ID
	PurchaseType  string `json:"purchase_type"`  // "subscription" or "consumable"
	PurchaseToken string `json:"purchase_token"` // Google: purchase token, Apple: base64 receipt or transaction ID
	OrderID       string `json:"order_id"`       // Google: orderId
	TransactionID string `json:"transaction_id"` // Apple: transactionId
}

func HandleIAPVerify(c *fiber.Ctx) error {
	var req IAPVerifyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Platform == "" || req.ProductID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "platform and product_id are required"})
	}

	// Get user/tenant from JWT
	userID, _ := c.Locals("user_id").(uuid.UUID)
	tenantID, _ := c.Locals("tenant_id").(uuid.UUID)
	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "unauthorized"})
	}

	// Look up the plan by IAP product ID
	var plan models.SubscriptionPlan
	var planQuery string
	switch req.Platform {
	case "google":
		planQuery = "google_product_id = ?"
	case "apple":
		planQuery = "apple_product_id = ?"
	default:
		return c.Status(400).JSON(fiber.Map{"error": "invalid platform, must be 'google' or 'apple'"})
	}

	if err := database.DB.Where(planQuery, req.ProductID).First(&plan).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "unknown product_id: " + req.ProductID})
	}

	switch req.Platform {
	case "google":
		return handleGoogleIAPVerify(c, &req, &plan, userID, tenantID)
	case "apple":
		return handleAppleIAPVerify(c, &req, &plan, userID, tenantID)
	default:
		return c.Status(400).JSON(fiber.Map{"error": "invalid platform"})
	}
}

// ──────────────────────────────────────────────────────────────────
// Google Play IAP Verification
// ──────────────────────────────────────────────────────────────────

func handleGoogleIAPVerify(c *fiber.Ctx, req *IAPVerifyRequest, plan *models.SubscriptionPlan, userID, tenantID uuid.UUID) error {
	cfg := iapConfig()
	if cfg.GoogleServiceAccountJSON == "" || cfg.GooglePackageName == "" {
		return c.Status(500).JSON(fiber.Map{"error": "Google Play IAP not configured"})
	}

	if req.PurchaseToken == "" {
		return c.Status(400).JSON(fiber.Map{"error": "purchase_token is required for Google Play"})
	}

	// Create Android Publisher client using service account
	ctx := c.Context()
	creds, err := google.CredentialsFromJSON(ctx, []byte(cfg.GoogleServiceAccountJSON),
		androidpublisher.AndroidpublisherScope)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create Google credentials"})
	}

	service, err := androidpublisher.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create Android Publisher service"})
	}

	if req.PurchaseType == models.IAPTypeSubscription {
		// Verify subscription purchase
		sub, err := service.Purchases.Subscriptionsv2.Get(
			cfg.GooglePackageName, req.PurchaseToken).Do()
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Google verification failed: " + err.Error()})
		}

		// Check subscription is valid
		if sub.SubscriptionState != "SUBSCRIPTION_STATE_ACTIVE" &&
			sub.SubscriptionState != "SUBSCRIPTION_STATE_IN_GRACE_PERIOD" {
			return c.Status(400).JSON(fiber.Map{
				"error":  "subscription not active",
				"status": sub.SubscriptionState,
			})
		}

		// Determine period from line items
		var expiryTime time.Time
		var startTime time.Time
		if len(sub.LineItems) > 0 {
			item := sub.LineItems[0]
			if item.ExpiryTime != "" {
				expiryTime, _ = time.Parse(time.RFC3339, item.ExpiryTime)
			}
		}
		if sub.StartTime != "" {
			startTime, _ = time.Parse(time.RFC3339, sub.StartTime)
		}

		// Save IAP purchase record
		rawBytes, _ := json.Marshal(sub)
		now := time.Now()
		purchase := models.IAPPurchase{
			TenantID:            tenantID,
			UserID:              userID,
			Platform:            models.PaymentProviderGoogle,
			ProductID:           req.ProductID,
			PurchaseType:        models.IAPTypeSubscription,
			GooglePurchaseToken: req.PurchaseToken,
			GoogleOrderID:       req.OrderID,
			Status:              models.IAPStatusPurchased,
			VerifiedAt:          &now,
			ExpiresAt:           &expiryTime,
			RawReceipt:          string(rawBytes),
		}
		database.DB.Create(&purchase)

		// Upsert subscription record
		upsertIAPSubscription(tenantID, plan, models.PaymentProviderGoogle,
			req.PurchaseToken, req.OrderID, "", "", "active", &startTime, &expiryTime)

		// Update tenant plan
		updateTenantPlan(tenantID, plan.Name, "active")

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Google Play subscription verified",
			"plan":    plan.Name,
			"status":  "active",
			"expires": expiryTime.Format(time.RFC3339),
		})

	} else {
		// Consumable product (e.g., AI Coins)
		prod, err := service.Purchases.Products.Get(
			cfg.GooglePackageName, req.ProductID, req.PurchaseToken).Do()
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Google verification failed: " + err.Error()})
		}

		if prod.PurchaseState != 0 {
			return c.Status(400).JSON(fiber.Map{"error": "purchase not completed"})
		}

		// Acknowledge the purchase
		ackReq := &androidpublisher.ProductPurchasesAcknowledgeRequest{}
		err = service.Purchases.Products.Acknowledge(
			cfg.GooglePackageName, req.ProductID, req.PurchaseToken, ackReq).Do()
		if err != nil {
			fmt.Printf("WARNING: failed to acknowledge Google purchase: %v\n", err)
		}

		// Save purchase record
		rawBytes, _ := json.Marshal(prod)
		now := time.Now()
		purchase := models.IAPPurchase{
			TenantID:            tenantID,
			UserID:              userID,
			Platform:            models.PaymentProviderGoogle,
			ProductID:           req.ProductID,
			PurchaseType:        models.IAPTypeConsumable,
			GooglePurchaseToken: req.PurchaseToken,
			GoogleOrderID:       req.OrderID,
			Status:              models.IAPStatusPurchased,
			VerifiedAt:          &now,
			RawReceipt:          string(rawBytes),
		}
		database.DB.Create(&purchase)

		// Grant the consumable (e.g., AI Coins) — call existing handler
		grantConsumable(tenantID, userID, req.ProductID)

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Google Play purchase verified and granted",
		})
	}
}

// ──────────────────────────────────────────────────────────────────
// Apple App Store IAP Verification (App Store Server API v2)
// ──────────────────────────────────────────────────────────────────

func handleAppleIAPVerify(c *fiber.Ctx, req *IAPVerifyRequest, plan *models.SubscriptionPlan, userID, tenantID uuid.UUID) error {
	cfg := iapConfig()
	if cfg.AppleIssuerID == "" || cfg.AppleKeyID == "" || cfg.ApplePrivateKey == "" {
		return c.Status(500).JSON(fiber.Map{"error": "Apple IAP not configured"})
	}

	if req.TransactionID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "transaction_id is required for Apple IAP"})
	}

	// Generate App Store Server API JWT
	token, err := generateAppleJWT(cfg.AppleIssuerID, cfg.AppleKeyID, cfg.ApplePrivateKey, cfg.AppleBundleID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to generate Apple API token"})
	}

	// Get transaction info from App Store Server API
	baseURL := "https://api.storekit.itunes.apple.com"
	if cfg.AppleEnvironment == "Sandbox" {
		baseURL = "https://api.storekit-sandbox.itunes.apple.com"
	}

	txnURL := fmt.Sprintf("%s/inApps/v1/transactions/%s", baseURL, req.TransactionID)
	httpReq, _ := http.NewRequest("GET", txnURL, nil)
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to verify with Apple: " + err.Error()})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return c.Status(400).JSON(fiber.Map{
			"error":  "Apple verification failed",
			"status": resp.StatusCode,
			"detail": string(body),
		})
	}

	var txnResponse struct {
		SignedTransactionInfo string `json:"signedTransactionInfo"`
	}
	if err := json.Unmarshal(body, &txnResponse); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to parse Apple response"})
	}

	// Decode the JWS (we trust Apple's signature since we called their API directly)
	txnInfo, err := decodeAppleJWSPayload(txnResponse.SignedTransactionInfo)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to decode transaction info"})
	}

	// Verify the product matches
	appleProductID := txnInfo["productId"]
	if appleProductID != req.ProductID {
		return c.Status(400).JSON(fiber.Map{
			"error":    "product_id mismatch",
			"expected": req.ProductID,
			"got":      appleProductID,
		})
	}

	// Verify bundle ID
	if bundleID, ok := txnInfo["bundleId"]; ok && bundleID != cfg.AppleBundleID {
		return c.Status(400).JSON(fiber.Map{
			"error": "bundle_id mismatch",
		})
	}

	originalTxnID, _ := txnInfo["originalTransactionId"].(string)
	transactionID, _ := txnInfo["transactionId"].(string)
	environment, _ := txnInfo["environment"].(string)

	now := time.Now()

	if req.PurchaseType == models.IAPTypeSubscription {
		// Parse expiry
		var expiresAt time.Time
		if exp, ok := txnInfo["expiresDate"]; ok {
			if expMs, ok := exp.(float64); ok {
				expiresAt = time.UnixMilli(int64(expMs))
			}
		}
		var purchaseDate time.Time
		if pd, ok := txnInfo["purchaseDate"]; ok {
			if pdMs, ok := pd.(float64); ok {
				purchaseDate = time.UnixMilli(int64(pdMs))
			}
		}

		// Save IAP purchase record
		rawBytes, _ := json.Marshal(txnInfo)
		purchase := models.IAPPurchase{
			TenantID:                   tenantID,
			UserID:                     userID,
			Platform:                   models.PaymentProviderApple,
			ProductID:                  req.ProductID,
			PurchaseType:               models.IAPTypeSubscription,
			AppleTransactionID:         transactionID,
			AppleOriginalTransactionID: originalTxnID,
			AppleEnvironment:           environment,
			Status:                     models.IAPStatusPurchased,
			VerifiedAt:                 &now,
			ExpiresAt:                  &expiresAt,
			RawReceipt:                 string(rawBytes),
		}
		database.DB.Create(&purchase)

		// Upsert subscription record
		upsertIAPSubscription(tenantID, plan, models.PaymentProviderApple,
			"", "", originalTxnID, transactionID, "active", &purchaseDate, &expiresAt)

		// Update tenant plan
		updateTenantPlan(tenantID, plan.Name, "active")

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Apple subscription verified",
			"plan":    plan.Name,
			"status":  "active",
			"expires": expiresAt.Format(time.RFC3339),
		})

	} else {
		// Consumable
		rawBytes, _ := json.Marshal(txnInfo)
		purchase := models.IAPPurchase{
			TenantID:                   tenantID,
			UserID:                     userID,
			Platform:                   models.PaymentProviderApple,
			ProductID:                  req.ProductID,
			PurchaseType:               models.IAPTypeConsumable,
			AppleTransactionID:         transactionID,
			AppleOriginalTransactionID: originalTxnID,
			AppleEnvironment:           environment,
			Status:                     models.IAPStatusPurchased,
			VerifiedAt:                 &now,
			RawReceipt:                 string(rawBytes),
		}
		database.DB.Create(&purchase)

		grantConsumable(tenantID, userID, req.ProductID)

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Apple purchase verified and granted",
		})
	}
}

// ──────────────────────────────────────────────────────────────────
// Google Play RTDN (Real-Time Developer Notifications) Webhook
// POST /api/v1/billing/webhook/google
// ──────────────────────────────────────────────────────────────────

func HandleGooglePlayWebhook(c *fiber.Ctx) error {
	cfg := iapConfig()
	if cfg.GoogleServiceAccountJSON == "" || cfg.GooglePackageName == "" {
		return c.SendStatus(200) // silently ignore if not configured
	}

	// Google Cloud Pub/Sub sends a POST with a message wrapper
	var pubsubMsg struct {
		Message struct {
			Data string `json:"data"` // base64 encoded
		} `json:"message"`
		Subscription string `json:"subscription"`
	}
	if err := c.BodyParser(&pubsubMsg); err != nil {
		return c.Status(400).SendString("invalid pubsub message")
	}

	data, err := base64.StdEncoding.DecodeString(pubsubMsg.Message.Data)
	if err != nil {
		return c.Status(400).SendString("invalid base64 data")
	}

	var notification struct {
		Version                  string `json:"version"`
		PackageName              string `json:"packageName"`
		EventTimeMillis          string `json:"eventTimeMillis"`
		SubscriptionNotification *struct {
			Version          string `json:"version"`
			NotificationType int    `json:"notificationType"`
			PurchaseToken    string `json:"purchaseToken"`
			SubscriptionID   string `json:"subscriptionId"`
		} `json:"subscriptionNotification"`
		OneTimeProductNotification *struct {
			Version          string `json:"version"`
			NotificationType int    `json:"notificationType"`
			PurchaseToken    string `json:"purchaseToken"`
			SKU              string `json:"sku"`
		} `json:"oneTimeProductNotification"`
	}
	if err := json.Unmarshal(data, &notification); err != nil {
		return c.Status(400).SendString("invalid notification data")
	}

	// Verify package name
	if notification.PackageName != cfg.GooglePackageName {
		return c.SendStatus(200) // ignore other packages
	}

	if notification.SubscriptionNotification != nil {
		return handleGoogleSubscriptionNotification(c, notification.SubscriptionNotification, cfg.GoogleServiceAccountJSON, cfg.GooglePackageName)
	}

	// OneTimeProduct notifications (consumables) — just log, no action needed
	return c.SendStatus(200)
}

func handleGoogleSubscriptionNotification(c *fiber.Ctx, notif *struct {
	Version          string `json:"version"`
	NotificationType int    `json:"notificationType"`
	PurchaseToken    string `json:"purchaseToken"`
	SubscriptionID   string `json:"subscriptionId"`
}, serviceAccountJSON, packageName string) error {

	// Notification types:
	// 1=RECOVERED, 2=RENEWED, 3=CANCELED, 4=PURCHASED,
	// 5=ON_HOLD, 6=IN_GRACE_PERIOD, 7=RESTARTED,
	// 12=REVOKED, 13=EXPIRED
	ctx := c.Context()
	creds, err := google.CredentialsFromJSON(ctx, []byte(serviceAccountJSON),
		androidpublisher.AndroidpublisherScope)
	if err != nil {
		return c.SendStatus(200)
	}
	service, err := androidpublisher.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return c.SendStatus(200)
	}

	// Get latest subscription state
	sub, err := service.Purchases.Subscriptionsv2.Get(
		packageName, notif.PurchaseToken).Do()
	if err != nil {
		fmt.Printf("Google RTDN: failed to get subscription: %v\n", err)
		return c.SendStatus(200)
	}

	// Find existing subscription by purchase token
	var existing models.Subscription
	if err := database.DB.Where("google_purchase_token = ? AND payment_provider = ?",
		notif.PurchaseToken, models.PaymentProviderGoogle).First(&existing).Error; err != nil {
		fmt.Printf("Google RTDN: subscription not found for token, ignoring\n")
		return c.SendStatus(200)
	}

	// Map Google state to our status
	status := "active"
	switch sub.SubscriptionState {
	case "SUBSCRIPTION_STATE_ACTIVE":
		status = "active"
	case "SUBSCRIPTION_STATE_CANCELED":
		status = "canceled"
	case "SUBSCRIPTION_STATE_EXPIRED":
		status = "canceled"
	case "SUBSCRIPTION_STATE_ON_HOLD":
		status = "past_due"
	case "SUBSCRIPTION_STATE_IN_GRACE_PERIOD":
		status = "active" // still active during grace period
	case "SUBSCRIPTION_STATE_PAUSED":
		status = "canceled"
	case "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED":
		status = "canceled"
	}

	// Update subscription
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if len(sub.LineItems) > 0 {
		item := sub.LineItems[0]
		if item.ExpiryTime != "" {
			if t, err := time.Parse(time.RFC3339, item.ExpiryTime); err == nil {
				updates["current_period_end"] = t
			}
		}
	}
	database.DB.Model(&existing).Updates(updates)

	// Update tenant status
	tenantStatus := "active"
	if status == "canceled" || status == "past_due" {
		tenantStatus = "suspended"
	}
	updateTenantPlan(existing.TenantID, "", tenantStatus)

	return c.SendStatus(200)
}

// ──────────────────────────────────────────────────────────────────
// App Store Server Notifications V2 Webhook
// POST /api/v1/billing/webhook/apple
// ──────────────────────────────────────────────────────────────────

func HandleAppleWebhook(c *fiber.Ctx) error {
	cfg := iapConfig()

	var body struct {
		SignedPayload string `json:"signedPayload"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).SendString("invalid request body")
	}

	if body.SignedPayload == "" {
		return c.Status(400).SendString("missing signedPayload")
	}

	// Decode the JWS payload (trust Apple's signature — their server sends it)
	payload, err := decodeAppleJWSPayload(body.SignedPayload)
	if err != nil {
		return c.Status(400).SendString("invalid signedPayload")
	}

	notificationType, _ := payload["notificationType"].(string)
	subtype, _ := payload["subtype"].(string)

	// Get transaction info from the notification data
	var txnInfo map[string]interface{}
	if data, ok := payload["data"].(map[string]interface{}); ok {
		if signedTxn, ok := data["signedTransactionInfo"].(string); ok {
			txnInfo, _ = decodeAppleJWSPayload(signedTxn)
		}
		// Also get renewal info if present
		if signedRenewal, ok := data["signedRenewalInfo"].(string); ok {
			renewalInfo, _ := decodeAppleJWSPayload(signedRenewal)
			_ = renewalInfo // available for future use
		}
	}

	if txnInfo == nil {
		return c.SendStatus(200) // can't process without transaction info
	}

	originalTxnID, _ := txnInfo["originalTransactionId"].(string)
	if originalTxnID == "" {
		return c.SendStatus(200)
	}

	// Verify bundle ID
	if bundleID, ok := txnInfo["bundleId"].(string); ok && cfg.AppleBundleID != "" && bundleID != cfg.AppleBundleID {
		return c.SendStatus(200) // ignore other apps
	}

	// Find existing subscription
	var existing models.Subscription
	if err := database.DB.Where("apple_original_transaction_id = ? AND payment_provider = ?",
		originalTxnID, models.PaymentProviderApple).First(&existing).Error; err != nil {
		fmt.Printf("Apple webhook: subscription not found for original_txn_id=%s\n", originalTxnID)
		return c.SendStatus(200)
	}

	// Map Apple notification type to subscription status
	// Reference: https://developer.apple.com/documentation/appstoreservernotifications/notificationtype
	status := existing.Status
	switch notificationType {
	case "DID_RENEW":
		status = "active"
	case "SUBSCRIBED":
		status = "active"
	case "DID_CHANGE_RENEWAL_STATUS":
		if subtype == "AUTO_RENEW_DISABLED" {
			// Will cancel at end of period, but still active
			database.DB.Model(&existing).Update("cancel_at_period_end", true)
		} else if subtype == "AUTO_RENEW_ENABLED" {
			database.DB.Model(&existing).Update("cancel_at_period_end", false)
		}
	case "EXPIRED":
		status = "canceled"
	case "DID_FAIL_TO_RENEW":
		if subtype == "GRACE_PERIOD" {
			status = "active" // grace period
		} else {
			status = "past_due"
		}
	case "GRACE_PERIOD_EXPIRED":
		status = "past_due"
	case "REVOKE":
		status = "canceled"
	case "REFUND":
		status = "canceled"
	}

	// Update subscription
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}

	// Update expiry from transaction info
	if exp, ok := txnInfo["expiresDate"]; ok {
		if expMs, ok := exp.(float64); ok {
			t := time.UnixMilli(int64(expMs))
			updates["current_period_end"] = t
		}
	}
	// Update transaction ID (may be new after renewal)
	if newTxnID, ok := txnInfo["transactionId"].(string); ok && newTxnID != "" {
		updates["apple_transaction_id"] = newTxnID
	}

	database.DB.Model(&existing).Updates(updates)

	// Update tenant status
	tenantStatus := "active"
	if status == "canceled" || status == "past_due" {
		tenantStatus = "suspended"
	}
	updateTenantPlan(existing.TenantID, "", tenantStatus)

	return c.SendStatus(200)
}

// ──────────────────────────────────────────────────────────────────
// GET /api/v1/billing/iap/products — Return IAP product IDs for client
// ──────────────────────────────────────────────────────────────────

func HandleGetIAPProducts(c *fiber.Ctx) error {
	var plans []models.SubscriptionPlan
	database.DB.Where("is_active = ?", true).
		Where("google_product_id IS NOT NULL OR apple_product_id IS NOT NULL").
		Order("price ASC").
		Find(&plans)

	products := make([]fiber.Map, 0, len(plans))
	for _, p := range plans {
		products = append(products, fiber.Map{
			"name":              p.Name,
			"display_name":      p.DisplayName,
			"price":             p.Price,
			"yearly_price":      p.YearlyPrice,
			"interval":          p.Interval,
			"google_product_id": p.GoogleProductID,
			"apple_product_id":  p.AppleProductID,
		})
	}

	return c.JSON(fiber.Map{"products": products})
}

// ──────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────

// upsertIAPSubscription creates or updates a subscription record for IAP
func upsertIAPSubscription(tenantID uuid.UUID, plan *models.SubscriptionPlan, provider,
	googleToken, googleOrderID, appleOriginalTxnID, appleTxnID, status string,
	periodStart, periodEnd *time.Time) {

	var existing models.Subscription
	var query string
	var args []interface{}

	switch provider {
	case models.PaymentProviderGoogle:
		query = "google_purchase_token = ? AND payment_provider = ?"
		args = []interface{}{googleToken, provider}
	case models.PaymentProviderApple:
		query = "apple_original_transaction_id = ? AND payment_provider = ?"
		args = []interface{}{appleOriginalTxnID, provider}
	default:
		return
	}

	err := database.DB.Where(query, args...).First(&existing).Error
	if err != nil {
		// Create new
		rec := models.Subscription{
			TenantID:                   tenantID,
			PlanID:                     plan.ID,
			PaymentProvider:            provider,
			GooglePurchaseToken:        googleToken,
			GoogleOrderID:              googleOrderID,
			AppleOriginalTransactionID: appleOriginalTxnID,
			AppleTransactionID:         appleTxnID,
			Status:                     status,
			CurrentPeriodStart:         periodStart,
			CurrentPeriodEnd:           periodEnd,
		}
		database.DB.Create(&rec)
	} else {
		// Update existing
		updates := map[string]interface{}{
			"status":               status,
			"plan_id":              plan.ID,
			"current_period_start": periodStart,
			"current_period_end":   periodEnd,
			"updated_at":           time.Now(),
		}
		if appleTxnID != "" {
			updates["apple_transaction_id"] = appleTxnID
		}
		database.DB.Model(&existing).Updates(updates)
	}
}

// updateTenantPlan updates the tenant's plan and status
func updateTenantPlan(tenantID uuid.UUID, planName, status string) {
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if planName != "" {
		updates["plan"] = planName
	}
	if status != "" {
		// Map subscription status to tenant status
		switch status {
		case "active", "trialing":
			updates["status"] = "active"
		case "canceled", "past_due":
			updates["status"] = "suspended"
		}
	}
	database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(updates)
}

// grantConsumable grants consumable IAP items (e.g., AI Coins)
func grantConsumable(tenantID, userID uuid.UUID, productID string) {
	// Map IAP product IDs to AI Coin amounts
	coinMap := map[string]int{
		// Google Play consumable product IDs
		"vai_coins_100":  100,
		"vai_coins_500":  500,
		"vai_coins_1000": 1000,
		"vai_coins_5000": 5000,
		// Apple consumable product IDs
		"com.vsys.vai.coins.100":  100,
		"com.vsys.vai.coins.500":  500,
		"com.vsys.vai.coins.1000": 1000,
		"com.vsys.vai.coins.5000": 5000,
	}

	amount, ok := coinMap[productID]
	if !ok {
		fmt.Printf("WARNING: unknown consumable product_id: %s\n", productID)
		return
	}

	// Credit AI Coins to tenant — reuse existing billing logic
	database.DB.Exec(`
		UPDATE tenants SET ai_coins_balance = COALESCE(ai_coins_balance, 0) + ?
		WHERE id = ?`, amount, tenantID)

	// Create transaction record
	database.DB.Exec(`
		INSERT INTO ai_coin_transactions (id, tenant_id, user_id, amount, type, description, created_at)
		VALUES (gen_random_uuid(), ?, ?, ?, 'credit', 'IAP Purchase', NOW())`,
		tenantID, userID, amount)
}

// generateAppleJWT generates a JWT for App Store Server API authentication
func generateAppleJWT(issuerID, keyID, privateKeyPEM, bundleID string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("private key is not ECDSA")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": issuerID,
		"iat": now.Unix(),
		"exp": now.Add(20 * time.Minute).Unix(),
		"aud": "appstoreconnect-v1",
		"bid": bundleID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = keyID

	return token.SignedString(ecKey)
}

// decodeAppleJWSPayload decodes the payload from an Apple JWS token (without full verification)
// We trust Apple's JWS when received directly from Apple's API or webhook
func decodeAppleJWSPayload(jws string) (map[string]interface{}, error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWS format")
	}

	// Add padding if needed
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Try without padding
		decoded, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("failed to decode payload: %w", err)
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(decoded, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return result, nil
}
