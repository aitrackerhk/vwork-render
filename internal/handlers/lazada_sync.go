package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"nwork/internal/services/lazada"
)

// GetLazadaAuthURL returns the OAuth authorization URL
func GetLazadaAuthURL(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	settings, err := lazada.Sync.GetLazadaSettings(tenantID)
	if err != nil || settings == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Lazada not configured. Please add App Key and App Secret first."})
	}

	client := lazada.NewClient(settings.AppKey, settings.AppSecret, "", "", settings.Region)

	// Build redirect URI
	scheme := "https"
	if c.Protocol() == "http" {
		scheme = "http"
	}
	redirectURI := fmt.Sprintf("%s://%s/api/v1/lazada/callback", scheme, c.Hostname())

	// Use tenant ID as state for security
	state := tenantID.String()

	authURL := client.GenerateAuthURL(redirectURI, state)

	return c.JSON(fiber.Map{
		"auth_url": authURL,
	})
}

// LazadaCallback handles OAuth callback
func LazadaCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing authorization code"})
	}

	// Parse tenant ID from state
	tenantID, err := uuid.Parse(state)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid state parameter"})
	}

	settings, err := lazada.Sync.GetLazadaSettings(tenantID)
	if err != nil || settings == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Lazada configuration not found"})
	}

	client := lazada.NewClient(settings.AppKey, settings.AppSecret, "", "", settings.Region)

	// Exchange code for tokens
	tokenResp, err := client.ExchangeAuthCode(code)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to exchange auth code: %v", err)})
	}

	// Update settings with tokens
	settings.AccessToken = tokenResp.AccessToken
	settings.RefreshToken = tokenResp.RefreshToken
	settings.SellerID = tokenResp.SellerID
	settings.Enabled = true

	if err := lazada.Sync.SaveLazadaSettings(tenantID, settings); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
	}

	// Redirect to settings page with success message
	redirectURL := "/product-sync-settings?lazada=connected"
	return c.Redirect(redirectURL)
}

// RefreshLazadaToken refreshes the access token
func RefreshLazadaToken(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	client, err := lazada.Sync.GetLazadaClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get Lazada client: %v", err)})
	}

	_, err = client.RefreshAccessToken()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to refresh token: %v", err)})
	}

	// Save updated tokens
	settings, _ := lazada.Sync.GetLazadaSettings(tenantID)
	settings.AccessToken = client.AccessToken
	settings.RefreshToken = client.RefreshToken
	lazada.Sync.SaveLazadaSettings(tenantID, settings)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Token refreshed successfully",
	})
}

// GetLazadaSellerInfo returns seller information
func GetLazadaSellerInfo(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	client, err := lazada.Sync.GetLazadaClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get Lazada client: %v", err)})
	}

	seller, err := client.GetSeller()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get seller info: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    seller,
	})
}

// GetLazadaProducts returns products from Lazada
func GetLazadaProducts(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	client, err := lazada.Sync.GetLazadaClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get Lazada client: %v", err)})
	}

	filter := c.Query("filter", "all")
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	products, err := client.GetProducts(filter, offset, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get products: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    products,
	})
}

// GetLazadaOrders returns orders from Lazada
func GetLazadaOrders(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	client, err := lazada.Sync.GetLazadaClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get Lazada client: %v", err)})
	}

	status := c.Query("status", "")
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	hours, _ := strconv.Atoi(c.Query("hours", "72"))

	createdAfter := time.Now().Add(-time.Duration(hours) * time.Hour)
	orders, err := client.GetOrders(createdAfter, time.Time{}, status, offset, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get orders: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    orders,
	})
}

// GetLazadaOrderDetail returns order details
func GetLazadaOrderDetail(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	orderIDStr := c.Params("order_id")
	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order ID"})
	}

	client, err := lazada.Sync.GetLazadaClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get Lazada client: %v", err)})
	}

	order, err := client.GetOrder(orderID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get order: %v", err)})
	}

	items, err := client.GetOrderItems(orderID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get order items: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"order": order,
			"items": items,
		},
	})
}

// SyncLazadaInventory syncs inventory to Lazada
func SyncLazadaInventory(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	var req struct {
		ProductID uuid.UUID `json:"product_id"`
		Quantity  int       `json:"quantity"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if err := lazada.Sync.SyncInventoryToLazada(tenantID, req.ProductID, req.Quantity); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to sync inventory: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Inventory synced successfully",
	})
}

// SyncLazadaPrice syncs price to Lazada
func SyncLazadaPrice(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	var req struct {
		ProductID uuid.UUID `json:"product_id"`
		Price     float64   `json:"price"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if err := lazada.Sync.SyncPriceToLazada(tenantID, req.ProductID, req.Price); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to sync price: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Price synced successfully",
	})
}

// TestLazadaConnection tests the Lazada connection
func TestLazadaConnection(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	client, err := lazada.Sync.GetLazadaClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success":   false,
			"connected": false,
			"error":     fmt.Sprintf("Failed to get client: %v", err),
		})
	}

	seller, err := client.GetSeller()
	if err != nil {
		return c.JSON(fiber.Map{
			"success":   false,
			"connected": false,
			"error":     fmt.Sprintf("Failed to connect: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"connected": true,
		"seller":    seller,
	})
}

// SaveLazadaSettingsHandler saves Lazada settings
func SaveLazadaSettingsHandler(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	var req lazada.LazadaSettings
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Preserve existing tokens if not provided
	existing, _ := lazada.Sync.GetLazadaSettings(tenantID)
	if existing != nil {
		if req.AccessToken == "" {
			req.AccessToken = existing.AccessToken
		}
		if req.RefreshToken == "" {
			req.RefreshToken = existing.RefreshToken
		}
		if req.SellerID == "" {
			req.SellerID = existing.SellerID
		}
	}

	if err := lazada.Sync.SaveLazadaSettings(tenantID, &req); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to save settings: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Settings saved successfully",
	})
}

// GetLazadaSyncStatus returns sync status
func GetLazadaSyncStatus(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	settings, err := lazada.Sync.GetLazadaSettings(tenantID)
	if err != nil || settings == nil {
		return c.JSON(fiber.Map{
			"configured": false,
			"enabled":    false,
			"connected":  false,
		})
	}

	return c.JSON(fiber.Map{
		"configured":   settings.AppKey != "",
		"enabled":      settings.Enabled,
		"connected":    settings.AccessToken != "",
		"region":       settings.Region,
		"seller_id":    settings.SellerID,
		"shop_name":    settings.ShopName,
		"auto_sync":    settings.AutoSync,
		"last_sync_at": settings.LastSyncAt,
	})
}

// ManualLazadaSync triggers a manual sync for all products
func ManualLazadaSync(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	settings, err := lazada.Sync.GetLazadaSettings(tenantID)
	if err != nil || settings == nil || !settings.Enabled {
		return c.Status(400).JSON(fiber.Map{"error": "Lazada sync not enabled"})
	}

	// Update last sync time
	settings.LastSyncAt = time.Now().Format(time.RFC3339)
	lazada.Sync.SaveLazadaSettings(tenantID, settings)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Manual sync triggered",
	})
}

// GetLazadaCategories returns Lazada categories
func GetLazadaCategories(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	client, err := lazada.Sync.GetLazadaClient(tenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get Lazada client: %v", err)})
	}

	categories, err := client.GetCategories()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to get categories: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    categories,
	})
}

// DisconnectLazada disconnects Lazada integration
func DisconnectLazada(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	settings, err := lazada.Sync.GetLazadaSettings(tenantID)
	if err != nil || settings == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Lazada not configured"})
	}

	// Clear tokens and disable
	settings.AccessToken = ""
	settings.RefreshToken = ""
	settings.Enabled = false
	settings.SellerID = ""

	if err := lazada.Sync.SaveLazadaSettings(tenantID, settings); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to disconnect"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Lazada disconnected successfully",
	})
}

// init to avoid unused import
var _ = url.Values{}
