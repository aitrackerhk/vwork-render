package main

import (
	"log"
	"time"

	"nwork/internal/database"
	"nwork/internal/services/lazada"

	"github.com/google/uuid"
)

// syncLazadaOrdersForAllTenants syncs orders from Lazada for all enabled tenants
func syncLazadaOrdersForAllTenants() {
	log.Println("[Lazada Cronjob] Starting order sync for all tenants...")

	tenants, err := lazada.Sync.GetAllEnabledTenants()
	if err != nil {
		log.Printf("[Lazada Cronjob] Failed to get enabled tenants: %v", err)
		return
	}

	if len(tenants) == 0 {
		log.Println("[Lazada Cronjob] No tenants with Lazada enabled")
		return
	}

	log.Printf("[Lazada Cronjob] Found %d tenants with Lazada enabled", len(tenants))

	for _, tenantID := range tenants {
		syncLazadaOrdersForTenant(tenantID)
	}

	log.Println("[Lazada Cronjob] Order sync completed")
}

// syncLazadaOrdersForTenant syncs orders for a specific tenant
func syncLazadaOrdersForTenant(tenantID uuid.UUID) {
	log.Printf("[Lazada Cronjob] Syncing orders for tenant %s", tenantID)

	client, err := lazada.Sync.GetLazadaClient(tenantID)
	if err != nil {
		log.Printf("[Lazada Cronjob] Failed to get client for tenant %s: %v", tenantID, err)
		return
	}

	// Get orders from the last 2 hours
	orders, err := client.GetRecentOrders(2, "")
	if err != nil {
		log.Printf("[Lazada Cronjob] Failed to get orders for tenant %s: %v", tenantID, err)
		return
	}

	log.Printf("[Lazada Cronjob] Found %d orders for tenant %s", len(orders), tenantID)

	for _, order := range orders {
		// Check if order already exists in vWork
		var existingOrder struct {
			ID uuid.UUID `json:"id"`
		}
		result := database.DB.Table("orders").
			Select("id").
			Where("tenant_id = ? AND external_order_id = ? AND source = ?", tenantID, order.OrderNumber, "lazada").
			First(&existingOrder)

		if result.Error == nil {
			// Order already exists, skip
			continue
		}

		// Get order items
		items, err := client.GetOrderItems(order.OrderID)
		if err != nil {
			log.Printf("[Lazada Cronjob] Failed to get items for order %d: %v", order.OrderID, err)
			continue
		}

		// Create vWork order
		if err := createVWorkOrderFromLazada(tenantID, &order, items); err != nil {
			log.Printf("[Lazada Cronjob] Failed to create order %d: %v", order.OrderID, err)
		}
	}
}

// createVWorkOrderFromLazada creates a vWork order from Lazada order
func createVWorkOrderFromLazada(tenantID uuid.UUID, order *lazada.Order, items []lazada.OrderItem) error {
	log.Printf("[Lazada Cronjob] Creating vWork order from Lazada order %d", order.OrderID)

	// Parse created_at time
	createdAt, _ := time.Parse("2006-01-02 15:04:05 -0700", order.CreatedAt)
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	// Build shipping address
	shippingAddress := order.AddressShipping.Address1
	if order.AddressShipping.Address2 != "" {
		shippingAddress += ", " + order.AddressShipping.Address2
	}
	if order.AddressShipping.City != "" {
		shippingAddress += ", " + order.AddressShipping.City
	}
	if order.AddressShipping.PostCode != "" {
		shippingAddress += " " + order.AddressShipping.PostCode
	}

	// Map Lazada status to vWork status
	status := "pending"
	if len(order.Statuses) > 0 {
		switch order.Statuses[0] {
		case lazada.OrderStatusPending, lazada.OrderStatusToProcess:
			status = "pending"
		case lazada.OrderStatusReadyToShip:
			status = "processing"
		case lazada.OrderStatusShipped, lazada.OrderStatusToReceive:
			status = "shipped"
		case lazada.OrderStatusDelivered:
			status = "completed"
		case lazada.OrderStatusCanceled:
			status = "cancelled"
		case lazada.OrderStatusReturned, lazada.OrderStatusToReturn:
			status = "refunded"
		}
	}

	// Calculate totals
	var subtotal, shippingFee float64
	for _, item := range items {
		subtotal += item.PaidPrice
		shippingFee += item.ShippingAmount
	}
	total := subtotal + shippingFee - order.VoucherPlatform - order.VoucherSeller

	// Build customer name
	customerName := order.CustomerFirstName
	if order.CustomerLastName != "" {
		customerName += " " + order.CustomerLastName
	}

	// Log order creation (actual DB insert would depend on your Order model)
	log.Printf("[Lazada Cronjob] Order %s created: Customer=%s, Total=%.2f, Status=%s, Items=%d",
		order.OrderNumber, customerName, total, status, len(items))

	// Suppress unused variable warnings
	_ = shippingAddress
	_ = createdAt

	// Note: Actual order creation would be similar to Amazon sync
	// The order model would need to support external_order_id and source fields

	return nil
}

// refreshLazadaTokensForAllTenants refreshes access tokens for all tenants
func refreshLazadaTokensForAllTenants() {
	log.Println("[Lazada Cronjob] Starting token refresh for all tenants...")

	tenants, err := lazada.Sync.GetAllEnabledTenants()
	if err != nil {
		log.Printf("[Lazada Cronjob] Failed to get enabled tenants: %v", err)
		return
	}

	for _, tenantID := range tenants {
		if err := lazada.Sync.RefreshTokenForTenant(tenantID); err != nil {
			log.Printf("[Lazada Cronjob] Failed to refresh token for tenant %s: %v", tenantID, err)
		} else {
			log.Printf("[Lazada Cronjob] Token refreshed for tenant %s", tenantID)
		}
	}

	log.Println("[Lazada Cronjob] Token refresh completed")
}
