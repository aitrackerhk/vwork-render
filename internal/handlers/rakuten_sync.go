package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"nwork/internal/services/rakuten"
)

// RakutenGetAuthURL returns the Rakuten OAuth authorization URL
func RakutenGetAuthURL(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	settings, err := rakuten.GetTenantSettings(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get settings"})
	}

	client := rakuten.NewClient(settings.ServiceSecret, settings.LicenseKey, settings.ShopID, "", "")
	state := uuid.New().String()

	return c.JSON(fiber.Map{
		"auth_url": client.GetAuthURL(state),
		"state":    state,
	})
}

// RakutenCallback handles OAuth callback from Rakuten
func RakutenCallback(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	code := c.Query("code")
	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Authorization code required"})
	}

	settings, err := rakuten.GetTenantSettings(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get settings"})
	}

	client := rakuten.NewClient(settings.ServiceSecret, settings.LicenseKey, settings.ShopID, "", "")

	if err := client.ExchangeCode(code); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to exchange code: " + err.Error()})
	}

	// Save tokens
	settings.AccessToken = client.GetAccessToken()
	settings.RefreshToken = client.GetRefreshToken()
	settings.TokenExpiry = client.GetTokenExpiry().Unix()
	settings.Enabled = true

	if err := rakuten.SaveTenantSettings(tenantID, settings); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Rakuten connected successfully",
	})
}

// RakutenGetShopInfo returns Rakuten shop information
func RakutenGetShopInfo(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	shopInfo, err := syncService.GetClient().GetShopInfo()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get shop info: " + err.Error()})
	}

	return c.JSON(shopInfo)
}

// RakutenGetProducts returns products from Rakuten
func RakutenGetProducts(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	status := c.Query("status", "")

	items, err := syncService.GetClient().GetItems(offset, limit, status)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get products: " + err.Error()})
	}

	return c.JSON(items)
}

// RakutenGetOrders returns orders from Rakuten
func RakutenGetOrders(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Parse query parameters
	status := c.Query("status", "")
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	var startDate, endDate time.Time
	if s := c.Query("start_date"); s != "" {
		startDate, _ = time.Parse("2006-01-02", s)
	}
	if e := c.Query("end_date"); e != "" {
		endDate, _ = time.Parse("2006-01-02", e)
	}

	orders, err := syncService.GetClient().GetOrders(startDate, endDate, status, offset, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get orders: " + err.Error()})
	}

	return c.JSON(orders)
}

// RakutenGetGenres returns product genres/categories
func RakutenGetGenres(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	parentID, _ := strconv.ParseInt(c.Query("parent_id", "0"), 10, 64)

	genres, err := syncService.GetClient().GetGenres(parentID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to get genres: " + err.Error()})
	}

	return c.JSON(genres)
}

// RakutenSyncInventory syncs inventory to Rakuten
func RakutenSyncInventory(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	var req struct {
		ProductID uuid.UUID `json:"product_id"`
		Quantity  int       `json:"quantity"`
		VariantID string    `json:"variant_id,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if req.VariantID != "" {
		err = syncService.SyncVariantInventory(req.ProductID, req.VariantID, req.Quantity)
	} else {
		err = syncService.SyncInventory(req.ProductID, req.Quantity)
	}

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// RakutenSyncPrice syncs price to Rakuten
func RakutenSyncPrice(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	var req struct {
		ProductID uuid.UUID `json:"product_id"`
		Price     int       `json:"price"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if err := syncService.SyncPrice(req.ProductID, req.Price); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// RakutenRefreshToken refreshes the Rakuten access token
func RakutenRefreshToken(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if err := syncService.RefreshTokenIfNeeded(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Token refreshed"})
}

// RakutenSyncOrders syncs orders from Rakuten
func RakutenSyncOrders(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if err := syncService.SyncOrders(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Orders synced"})
}

// RakutenGetSyncStatus returns the sync status
func RakutenGetSyncStatus(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	status, err := syncService.GetSyncStatus()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(status)
}

// RakutenTestConnection tests the Rakuten connection
func RakutenTestConnection(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if err := syncService.TestConnection(); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"connected": false,
			"error":     err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"connected": true,
		"message":   "Connection successful",
	})
}

// RakutenUpdateSettings updates Rakuten settings
func RakutenUpdateSettings(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	var req struct {
		ServiceSecret string `json:"service_secret"`
		LicenseKey    string `json:"license_key"`
		ShopID        string `json:"shop_id"`
		Enabled       bool   `json:"enabled"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	settings, err := rakuten.GetTenantSettings(tenantID)
	if err != nil {
		settings = &rakuten.RakutenSettings{}
	}

	if req.ServiceSecret != "" {
		settings.ServiceSecret = req.ServiceSecret
	}
	if req.LicenseKey != "" {
		settings.LicenseKey = req.LicenseKey
	}
	if req.ShopID != "" {
		settings.ShopID = req.ShopID
	}
	settings.Enabled = req.Enabled

	if err := rakuten.SaveTenantSettings(tenantID, settings); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
	}

	return c.JSON(fiber.Map{"success": true})
}

// RakutenDisconnect disconnects Rakuten integration
func RakutenDisconnect(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	if err := rakuten.DisconnectRakuten(tenantID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Rakuten disconnected"})
}

// RakutenUpdateOrderStatus updates order status
func RakutenUpdateOrderStatus(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	orderNumber := c.Params("orderNumber")
	if orderNumber == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Order number required"})
	}

	var req struct {
		Status string `json:"status"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if err := syncService.GetClient().UpdateOrderStatus(orderNumber, req.Status); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

// RakutenShipOrder ships an order
func RakutenShipOrder(c *fiber.Ctx) error {
	tenantID, err := uuid.Parse(c.Locals("tenant_id").(string))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant ID"})
	}

	orderNumber := c.Params("orderNumber")
	if orderNumber == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Order number required"})
	}

	var req struct {
		DeliveryCompany string `json:"delivery_company"`
		TrackingNumber  string `json:"tracking_number"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if err := syncService.GetClient().ShipOrder(orderNumber, req.DeliveryCompany, req.TrackingNumber); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Order shipped"})
}
