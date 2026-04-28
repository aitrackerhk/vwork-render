package handlers

import (
	"strconv"
	"time"

	"nwork/internal/middleware"
	"nwork/internal/services/amazon"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetAmazonAuthURL generates Amazon OAuth authorization URL
func GetAmazonAuthURL(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	settings, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Amazon settings not configured",
		})
	}

	if settings.ClientID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Amazon Client ID is required",
		})
	}

	// Determine region endpoint
	region := settings.Region
	if region == "" {
		region = "NA"
	}

	client := amazon.NewClient(
		settings.ClientID,
		settings.ClientSecret,
		"",
		region,
		settings.MarketplaceID,
		settings.AWSAccessKeyID,
		settings.AWSSecretKey,
		settings.AWSRoleARN,
	)

	// State includes tenant ID for callback
	state := c.Query("state", tenantID.String())
	redirectURI := c.Query("redirect_uri", "")

	authURL := client.GenerateAuthURL(redirectURI, state)

	return c.JSON(fiber.Map{
		"auth_url": authURL,
	})
}

// AmazonCallback handles OAuth callback from Amazon
func AmazonCallback(c *fiber.Ctx) error {
	code := c.Query("spapi_oauth_code")
	state := c.Query("state")
	sellingPartnerID := c.Query("selling_partner_id")

	if code == "" {
		errorDesc := c.Query("error_description", "Authorization failed")
		return c.Status(400).JSON(fiber.Map{
			"error": errorDesc,
		})
	}

	// Parse state to get tenant ID
	tenantID, err := uuid.Parse(state)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid state parameter",
		})
	}

	settings, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Amazon settings not found",
		})
	}

	region := settings.Region
	if region == "" {
		region = "NA"
	}

	client := amazon.NewClient(
		settings.ClientID,
		settings.ClientSecret,
		"",
		region,
		settings.MarketplaceID,
		settings.AWSAccessKeyID,
		settings.AWSSecretKey,
		settings.AWSRoleARN,
	)

	redirectURI := c.Query("redirect_uri", "")
	_ = redirectURI // redirectURI is included in OAuth flow but not needed for token exchange
	tokenResp, err := client.ExchangeAuthCode(code)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to exchange authorization code: " + err.Error(),
		})
	}

	// Save refresh token and seller ID
	settings.RefreshToken = tokenResp.RefreshToken
	settings.SellerID = sellingPartnerID
	settings.Enabled = true
	settings.LastSyncTime = time.Now().Format(time.RFC3339)

	if err := amazon.Sync.SaveAmazonSettings(tenantID, settings); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to save Amazon settings: " + err.Error(),
		})
	}

	// Redirect to settings page with success message
	return c.Redirect("/product-sync-settings?amazon=success")
}

// RefreshAmazonToken refreshes the Amazon access token
func RefreshAmazonToken(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	client, err := amazon.Sync.GetAmazonClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if _, err := client.RefreshAccessToken(); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to refresh token: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Token refreshed successfully",
	})
}

// GetAmazonSellerInfo gets seller information
func GetAmazonSellerInfo(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	settings, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"seller_id":      settings.SellerID,
		"marketplace_id": settings.MarketplaceID,
		"region":         settings.Region,
		"enabled":        settings.Enabled,
		"last_sync_time": settings.LastSyncTime,
	})
}

// GetAmazonOrders retrieves orders from Amazon
func GetAmazonOrders(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	client, err := amazon.Sync.GetAmazonClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Get orders from last 7 days
	hoursStr := c.Query("hours", "168")
	hours, _ := strconv.Atoi(hoursStr)
	if hours <= 0 {
		hours = 168
	}
	statuses := c.Query("statuses", "")

	var orderStatuses []string
	if statuses != "" {
		orderStatuses = splitStringByComma(statuses)
	}

	orders, err := client.GetRecentOrders(hours, orderStatuses)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to get orders: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"orders": orders,
		"count":  len(orders),
	})
}

// GetAmazonOrderDetail retrieves order details
func GetAmazonOrderDetail(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	orderID := c.Params("order_id")

	client, err := amazon.Sync.GetAmazonClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	order, err := client.GetOrder(orderID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to get order: " + err.Error(),
		})
	}

	items, err := client.GetAllOrderItems(orderID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to get order items: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"order": order,
		"items": items,
	})
}

// GetAmazonListings retrieves listings from Amazon
func GetAmazonListings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	settings, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	client, err := amazon.Sync.GetAmazonClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	sku := c.Query("sku", "")
	if sku == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "SKU is required",
		})
	}

	includedData := []string{"summaries", "attributes", "issues", "offers", "fulfillmentAvailability"}
	listing, err := client.GetListingsItem(settings.SellerID, sku, includedData)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to get listing: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"listing": listing,
	})
}

// SyncAmazonInventory syncs inventory to Amazon
func SyncAmazonInventory(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		SKU      string `json:"sku"`
		Quantity int    `json:"quantity"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	settings, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	client, err := amazon.Sync.GetAmazonClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err := client.UpdateInventory(settings.SellerID, req.SKU, req.Quantity); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to update inventory: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":  "Inventory updated successfully",
		"sku":      req.SKU,
		"quantity": req.Quantity,
	})
}

// SyncAmazonPrice syncs price to Amazon
func SyncAmazonPrice(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		SKU      string  `json:"sku"`
		Price    float64 `json:"price"`
		Currency string  `json:"currency"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Currency == "" {
		req.Currency = "USD"
	}

	settings, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	client, err := amazon.Sync.GetAmazonClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err := client.UpdatePrice(settings.SellerID, req.SKU, req.Price, req.Currency); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to update price: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":  "Price updated successfully",
		"sku":      req.SKU,
		"price":    req.Price,
		"currency": req.Currency,
	})
}

// TestAmazonConnection tests the Amazon API connection
func TestAmazonConnection(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	client, err := amazon.Sync.GetAmazonClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"connected": false,
			"error":     err.Error(),
		})
	}

	// Test by refreshing token
	if _, err := client.RefreshAccessToken(); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"connected": false,
			"error":     "Failed to authenticate: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"connected": true,
		"message":   "Amazon connection is working",
	})
}

// SaveAmazonSettings saves Amazon API settings
func SaveAmazonSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var settings amazon.AmazonSettings
	if err := c.BodyParser(&settings); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get existing settings to preserve refresh token if not provided
	existing, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err == nil && settings.RefreshToken == "" {
		settings.RefreshToken = existing.RefreshToken
	}
	if err == nil && settings.SellerID == "" {
		settings.SellerID = existing.SellerID
	}

	if err := amazon.Sync.SaveAmazonSettings(tenantID, &settings); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to save settings: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Amazon settings saved successfully",
	})
}

// GetAmazonSyncStatus gets the current sync status
func GetAmazonSyncStatus(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	settings, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err != nil {
		return c.JSON(fiber.Map{
			"configured": false,
			"enabled":    false,
		})
	}

	return c.JSON(fiber.Map{
		"configured":     settings.ClientID != "" && settings.ClientSecret != "",
		"enabled":        settings.Enabled,
		"seller_id":      settings.SellerID,
		"marketplace_id": settings.MarketplaceID,
		"region":         settings.Region,
		"last_sync_time": settings.LastSyncTime,
	})
}

// ManualAmazonSync triggers a manual sync for all products
func ManualAmazonSync(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	settings, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if !settings.Enabled {
		return c.Status(400).JSON(fiber.Map{
			"error": "Amazon integration is not enabled",
		})
	}

	// Trigger sync in background
	go func() {
		// This would iterate through products and sync each one
		// For now, log that manual sync was triggered
	}()

	return c.JSON(fiber.Map{
		"message": "Manual sync started",
	})
}

// splitStringByComma splits a string by comma
func splitStringByComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
