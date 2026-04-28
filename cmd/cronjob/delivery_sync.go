package main

import (
	"log"
	"time"

	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/services/delivery"
)

// syncDeliveryOrdersForAllTenants pulls new orders from all enabled delivery platforms
func syncDeliveryOrdersForAllTenants() {
	log.Println("🛵 Running delivery order sync job at", time.Now().Format("2006-01-02 15:04:05"))

	var integrations []models.DeliveryIntegration
	if err := database.DB.
		Where("is_enabled = ? AND is_connected = ?", true, true).
		Find(&integrations).Error; err != nil {
		log.Printf("❌ Failed to query delivery integrations for order sync: %v", err)
		return
	}

	if len(integrations) == 0 {
		return
	}

	for _, integration := range integrations {
		syncDeliveryOrdersForIntegration(integration)
	}
}

func syncDeliveryOrdersForIntegration(integration models.DeliveryIntegration) {
	config := delivery.IntegrationConfig{
		Platform:     delivery.Platform(integration.Platform),
		MerchantID:   integration.MerchantID,
		APIKey:       integration.APIKey,
		APISecret:    integration.APISecret,
		AccessToken:  integration.AccessToken,
		RefreshToken: integration.RefreshToken,
	}

	service := delivery.NewIntegrationService(integration.TenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		log.Printf("❌ [DeliveryCron] Failed to create platform service for %s (tenant=%s): %v",
			integration.Platform, integration.TenantID, err)
		return
	}

	// Pull orders since last sync (or last 24 hours)
	since := time.Now().Add(-24 * time.Hour)
	if integration.LastSyncAt != nil {
		since = *integration.LastSyncAt
	}

	orders, err := platformService.GetOrders(since, 100)
	if err != nil {
		log.Printf("❌ [DeliveryCron] Failed to get orders from %s (tenant=%s): %v",
			integration.Platform, integration.TenantID, err)
		// Record error on integration
		integration.LastError = err.Error()
		database.DB.Save(&integration)
		return
	}

	if len(orders) > 0 {
		log.Printf("📦 [DeliveryCron] Got %d orders from %s for tenant %s",
			len(orders), integration.Platform, integration.TenantID)

		syncedCount := 0
		for _, pOrder := range orders {
			if err := createDeliveryOrderFromCron(integration.TenantID, &integration, &pOrder); err != nil {
				log.Printf("❌ [DeliveryCron] Failed to save order %s: %v", pOrder.PlatformOrderID, err)
			} else {
				syncedCount++
			}
		}
		log.Printf("✅ [DeliveryCron] Synced %d/%d orders from %s for tenant %s",
			syncedCount, len(orders), integration.Platform, integration.TenantID)
	}

	// Update last sync time
	now := time.Now()
	integration.LastSyncAt = &now
	integration.LastError = ""
	database.DB.Save(&integration)
}

// createDeliveryOrderFromCron creates or updates a delivery order from cron sync
func createDeliveryOrderFromCron(tenantID interface{}, integration *models.DeliveryIntegration, pOrder *delivery.Order) error {
	// Check if order already exists
	var existingCount int64
	database.DB.Model(&models.Order{}).
		Where("tenant_id = ? AND platform_order_id = ? AND source_type = ?",
			tenantID, pOrder.PlatformOrderID, "delivery").
		Count(&existingCount)

	if existingCount > 0 {
		return nil // Already synced
	}

	platform := string(pOrder.Platform)

	// Create the order
	order := models.Order{
		TenantID:         integration.TenantID,
		SourceType:       "delivery",
		DeliveryPlatform: &platform,
		PlatformOrderID:  &pOrder.PlatformOrderID,
		Status:           string(pOrder.Status),
		ContactName:      pOrder.CustomerName,
		ContactPhone:     pOrder.CustomerPhone,
		ContactAddress:   pOrder.CustomerAddress,
		Notes:            pOrder.CustomerNotes,
		TotalAmount:      pOrder.TotalAmount,
		OrderDate:        pOrder.CreatedAt,
	}

	if err := database.DB.Create(&order).Error; err != nil {
		return err
	}

	// Create delivery order detail
	detail := models.DeliveryOrderDetail{
		OrderID:               order.ID,
		IntegrationID:         &integration.ID,
		Platform:              models.DeliveryPlatform(platform),
		PlatformOrderID:       pOrder.PlatformOrderID,
		PlatformOrderNumber:   pOrder.PlatformOrderNumber,
		PlatformStatus:        pOrder.PlatformStatus,
		DeliveryType:          pOrder.DeliveryType,
		EstimatedPickupTime:   pOrder.EstimatedPickupTime,
		EstimatedDeliveryTime: pOrder.EstimatedDeliveryTime,
		RiderName:             pOrder.RiderName,
		RiderPhone:            pOrder.RiderPhone,
		RiderTrackingURL:      pOrder.RiderTrackingURL,
		PlatformFee:           pOrder.PlatformFee,
		DeliveryFee:           pOrder.DeliveryFee,
		PlatformDiscount:      pOrder.DiscountAmount,
	}

	if err := database.DB.Create(&detail).Error; err != nil {
		log.Printf("❌ [DeliveryCron] Failed to create delivery detail for order %s: %v", pOrder.PlatformOrderID, err)
	}

	return nil
}

// refreshDeliveryTokensForAllTenants refreshes tokens for all delivery integrations
func refreshDeliveryTokensForAllTenants() {
	log.Println("🔑 Running delivery token refresh job at", time.Now().Format("2006-01-02 15:04:05"))

	var integrations []models.DeliveryIntegration
	if err := database.DB.
		Where("is_enabled = ? AND is_connected = ?", true, true).
		Where("access_token IS NOT NULL AND access_token != ''").
		Find(&integrations).Error; err != nil {
		log.Printf("❌ Failed to query delivery integrations for token refresh: %v", err)
		return
	}

	if len(integrations) == 0 {
		return
	}

	for _, integration := range integrations {
		refreshDeliveryTokenForIntegration(integration)
	}
}

func refreshDeliveryTokenForIntegration(integration models.DeliveryIntegration) {
	// Skip if token is not expiring soon (more than 6 hours remaining)
	if integration.TokenExpiresAt != nil && time.Until(*integration.TokenExpiresAt) > 6*time.Hour {
		return
	}

	config := delivery.IntegrationConfig{
		Platform:     delivery.Platform(integration.Platform),
		MerchantID:   integration.MerchantID,
		APIKey:       integration.APIKey,
		APISecret:    integration.APISecret,
		AccessToken:  integration.AccessToken,
		RefreshToken: integration.RefreshToken,
	}

	service := delivery.NewIntegrationService(integration.TenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		log.Printf("❌ [DeliveryCron] Failed to create platform service for token refresh %s (tenant=%s): %v",
			integration.Platform, integration.TenantID, err)
		return
	}

	newAccessToken, newRefreshToken, expiresAt, err := platformService.RefreshToken()
	if err != nil {
		log.Printf("❌ [DeliveryCron] Failed to refresh token for %s (tenant=%s): %v",
			integration.Platform, integration.TenantID, err)
		integration.LastError = "Token refresh failed: " + err.Error()
		database.DB.Save(&integration)
		return
	}

	// Update tokens in database
	integration.AccessToken = newAccessToken
	if newRefreshToken != "" {
		integration.RefreshToken = newRefreshToken
	}
	if expiresAt != nil {
		integration.TokenExpiresAt = expiresAt
	}
	integration.LastError = ""

	if err := database.DB.Save(&integration).Error; err != nil {
		log.Printf("❌ [DeliveryCron] Failed to save refreshed tokens for %s (tenant=%s): %v",
			integration.Platform, integration.TenantID, err)
		return
	}

	log.Printf("✅ [DeliveryCron] Token refreshed for %s (tenant=%s)", integration.Platform, integration.TenantID)
}
