package main

import (
	"log"
	"time"

	"nwork/internal/database"
	"nwork/internal/services/amazon"

	"github.com/google/uuid"
)

// syncAmazonOrdersForAllTenants syncs orders from Amazon for all enabled tenants
// This runs every 15 minutes to check for new orders
func syncAmazonOrdersForAllTenants() {
	log.Println("[Amazon Cronjob] Starting order sync for all tenants...")

	tenants, err := amazon.Sync.GetAllEnabledTenants()
	if err != nil {
		log.Printf("[Amazon Cronjob] Failed to get enabled tenants: %v", err)
		return
	}

	log.Printf("[Amazon Cronjob] Found %d tenants with Amazon enabled", len(tenants))

	for _, tenantID := range tenants {
		syncAmazonOrdersForTenant(tenantID)
	}

	log.Println("[Amazon Cronjob] Order sync completed")
}

// syncAmazonOrdersForTenant syncs orders for a specific tenant
func syncAmazonOrdersForTenant(tenantID uuid.UUID) {
	log.Printf("[Amazon Cronjob] Syncing orders for tenant %s", tenantID)

	client, err := amazon.Sync.GetAmazonClient(tenantID)
	if err != nil {
		log.Printf("[Amazon Cronjob] Failed to get client for tenant %s: %v", tenantID, err)
		return
	}

	// Get orders from last 24 hours to catch any missed orders
	orderStatuses := []string{"Unshipped", "PartiallyShipped", "Shipped"}
	orders, err := client.GetRecentOrders(24, orderStatuses)
	if err != nil {
		log.Printf("[Amazon Cronjob] Failed to get orders for tenant %s: %v", tenantID, err)
		return
	}

	log.Printf("[Amazon Cronjob] Found %d orders for tenant %s", len(orders), tenantID)

	for _, order := range orders {
		// Check if order already exists in vWork
		var existingOrder struct {
			ID uuid.UUID `json:"id"`
		}
		result := database.DB.Table("orders").
			Select("id").
			Where("tenant_id = ? AND external_order_id = ? AND source = ?", tenantID, order.AmazonOrderID, "amazon").
			First(&existingOrder)

		if result.Error == nil {
			// Order already exists, skip
			continue
		}

		// Get order items
		items, err := client.GetAllOrderItems(order.AmazonOrderID)
		if err != nil {
			log.Printf("[Amazon Cronjob] Failed to get items for order %s: %v", order.AmazonOrderID, err)
			continue
		}

		// Create order in vWork
		if err := createVWorkOrderFromAmazon(tenantID, &order, items); err != nil {
			log.Printf("[Amazon Cronjob] Failed to create vWork order for %s: %v", order.AmazonOrderID, err)
			continue
		}

		log.Printf("[Amazon Cronjob] Created order %s for tenant %s", order.AmazonOrderID, tenantID)
	}

	// Update last sync time
	settings, err := amazon.Sync.GetAmazonSettings(tenantID)
	if err == nil {
		settings.LastSyncTime = time.Now().Format(time.RFC3339)
		amazon.Sync.SaveAmazonSettings(tenantID, settings)
	}
}

// createVWorkOrderFromAmazon creates a vWork order from an Amazon order
func createVWorkOrderFromAmazon(tenantID uuid.UUID, order *amazon.Order, items []amazon.OrderItem) error {
	// Map Amazon order status to vWork status
	status := mapAmazonStatusToVWork(order.OrderStatus)

	// Parse order date
	orderDate, _ := time.Parse(time.RFC3339, order.PurchaseDate)
	if orderDate.IsZero() {
		orderDate = time.Now()
	}

	// Build customer name from shipping address
	customerName := ""
	customerPhone := ""
	customerAddress := ""
	if order.ShippingAddress != nil {
		customerName = order.ShippingAddress.Name
		customerPhone = order.ShippingAddress.Phone
		customerAddress = buildAddressString(order.ShippingAddress)
	}

	// Calculate total from order total
	totalAmount := 0.0
	currency := "USD"
	if order.OrderTotal != nil {
		totalAmount = parseAmount(order.OrderTotal.Amount)
		currency = order.OrderTotal.CurrencyCode
	}

	// Generate new UUID for order
	orderID := uuid.New()

	// Create the order record
	orderRecord := map[string]interface{}{
		"id":                  orderID,
		"tenant_id":           tenantID,
		"external_order_id":   order.AmazonOrderID,
		"source":              "amazon",
		"status":              status,
		"order_date":          orderDate,
		"customer_name":       customerName,
		"customer_phone":      customerPhone,
		"shipping_address":    customerAddress,
		"total_amount":        totalAmount,
		"currency":            currency,
		"is_prime":            order.IsPrime,
		"fulfillment_channel": order.FulfillmentChannel,
		"created_at":          time.Now(),
		"updated_at":          time.Now(),
	}

	result := database.DB.Table("orders").Create(&orderRecord)
	if result.Error != nil {
		return result.Error
	}

	// Create order items
	for _, item := range items {
		itemPrice := 0.0
		if item.ItemPrice != nil {
			itemPrice = parseAmount(item.ItemPrice.Amount)
		}

		orderItemID := uuid.New()
		orderItem := map[string]interface{}{
			"id":               orderItemID,
			"order_id":         orderID,
			"tenant_id":        tenantID,
			"asin":             item.ASIN,
			"seller_sku":       item.SellerSKU,
			"title":            item.Title,
			"quantity_ordered": item.QuantityOrdered,
			"quantity_shipped": item.QuantityShipped,
			"item_price":       itemPrice,
			"created_at":       time.Now(),
		}

		// Try to find matching product by SKU
		var product struct {
			ID uuid.UUID `json:"id"`
		}
		if database.DB.Table("products").
			Where("tenant_id = ? AND sku = ?", tenantID, item.SellerSKU).
			First(&product).Error == nil {
			orderItem["product_id"] = product.ID
		}

		database.DB.Table("order_items").Create(&orderItem)
	}

	return nil
}

// mapAmazonStatusToVWork maps Amazon order status to vWork status
func mapAmazonStatusToVWork(amazonStatus string) string {
	statusMap := map[string]string{
		"Pending":             "pending",
		"Unshipped":           "confirmed",
		"PartiallyShipped":    "partially_shipped",
		"Shipped":             "shipped",
		"Canceled":            "cancelled",
		"Unfulfillable":       "unfulfillable",
		"InvoiceUnconfirmed":  "pending_invoice",
		"PendingAvailability": "pending",
	}

	if status, ok := statusMap[amazonStatus]; ok {
		return status
	}
	return "pending"
}

// buildAddressString builds a full address string from an Address
func buildAddressString(addr *amazon.Address) string {
	parts := []string{}
	if addr.AddressLine1 != "" {
		parts = append(parts, addr.AddressLine1)
	}
	if addr.AddressLine2 != "" {
		parts = append(parts, addr.AddressLine2)
	}
	if addr.AddressLine3 != "" {
		parts = append(parts, addr.AddressLine3)
	}
	if addr.City != "" {
		parts = append(parts, addr.City)
	}
	if addr.StateOrRegion != "" {
		parts = append(parts, addr.StateOrRegion)
	}
	if addr.PostalCode != "" {
		parts = append(parts, addr.PostalCode)
	}
	if addr.CountryCode != "" {
		parts = append(parts, addr.CountryCode)
	}

	result := ""
	for i, part := range parts {
		if i > 0 {
			result += ", "
		}
		result += part
	}
	return result
}

// parseAmount parses an amount string to float64
func parseAmount(amount string) float64 {
	var f float64
	for i := 0; i < len(amount); i++ {
		c := amount[i]
		if c >= '0' && c <= '9' {
			f = f*10 + float64(c-'0')
		} else if c == '.' {
			// Handle decimal part
			multiplier := 0.1
			for j := i + 1; j < len(amount); j++ {
				c := amount[j]
				if c >= '0' && c <= '9' {
					f += float64(c-'0') * multiplier
					multiplier *= 0.1
				}
			}
			break
		}
	}
	return f
}

// refreshAmazonTokensForAllTenants refreshes tokens for all enabled tenants
// This runs every 3 hours to ensure tokens stay valid
func refreshAmazonTokensForAllTenants() {
	log.Println("[Amazon Cronjob] Starting token refresh for all tenants...")

	tenants, err := amazon.Sync.GetAllEnabledTenants()
	if err != nil {
		log.Printf("[Amazon Cronjob] Failed to get enabled tenants: %v", err)
		return
	}

	for _, tenantID := range tenants {
		client, err := amazon.Sync.GetAmazonClient(tenantID)
		if err != nil {
			log.Printf("[Amazon Cronjob] Failed to get client for tenant %s: %v", tenantID, err)
			continue
		}

		if _, err := client.RefreshAccessToken(); err != nil {
			log.Printf("[Amazon Cronjob] Failed to refresh token for tenant %s: %v", tenantID, err)
		} else {
			log.Printf("[Amazon Cronjob] Refreshed token for tenant %s", tenantID)
		}
	}

	log.Println("[Amazon Cronjob] Token refresh completed")
}
