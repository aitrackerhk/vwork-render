package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"nwork/internal/database"
	"nwork/internal/services/rakuten"
)

// syncRakutenOrdersForAllTenants syncs orders from Rakuten for all enabled tenants
func syncRakutenOrdersForAllTenants() {
	log.Println("[Rakuten Cronjob] Starting order sync for all tenants...")

	tenants, err := getRakutenEnabledTenants()
	if err != nil {
		log.Printf("[Rakuten Cronjob] Failed to get tenants: %v", err)
		return
	}

	for _, tenantID := range tenants {
		if err := syncRakutenOrdersForTenant(tenantID); err != nil {
			log.Printf("[Rakuten Cronjob] Failed to sync orders for tenant %s: %v", tenantID, err)
		}
	}

	log.Println("[Rakuten Cronjob] Order sync completed")
}

// syncRakutenOrdersForTenant syncs orders for a single tenant
func syncRakutenOrdersForTenant(tenantID uuid.UUID) error {
	syncService, err := rakuten.NewSyncService(tenantID)
	if err != nil {
		return err
	}

	// Refresh token if needed
	if err := syncService.RefreshTokenIfNeeded(); err != nil {
		log.Printf("[Rakuten Cronjob] Token refresh failed for tenant %s: %v", tenantID, err)
	}

	return syncService.SyncOrders()
}

// refreshRakutenTokensForAllTenants refreshes access tokens for all enabled tenants
func refreshRakutenTokensForAllTenants() {
	log.Println("[Rakuten Cronjob] Starting token refresh for all tenants...")

	tenants, err := getRakutenEnabledTenants()
	if err != nil {
		log.Printf("[Rakuten Cronjob] Failed to get tenants: %v", err)
		return
	}

	for _, tenantID := range tenants {
		syncService, err := rakuten.NewSyncService(tenantID)
		if err != nil {
			log.Printf("[Rakuten Cronjob] Failed to create sync service for tenant %s: %v", tenantID, err)
			continue
		}

		if err := syncService.RefreshTokenIfNeeded(); err != nil {
			log.Printf("[Rakuten Cronjob] Token refresh failed for tenant %s: %v", tenantID, err)
		}
	}

	log.Println("[Rakuten Cronjob] Token refresh completed")
}

// getRakutenEnabledTenants returns all tenants with Rakuten sync enabled
func getRakutenEnabledTenants() ([]uuid.UUID, error) {
	type TenantRow struct {
		ID          uuid.UUID
		ExtraFields json.RawMessage
	}

	var rows []TenantRow
	err := database.DB.Table("tenants").
		Select("id, extra_fields").
		Find(&rows).Error

	if err != nil {
		return nil, err
	}

	var enabledTenants []uuid.UUID
	for _, row := range rows {
		if len(row.ExtraFields) == 0 {
			continue
		}

		var settings map[string]interface{}
		if err := json.Unmarshal(row.ExtraFields, &settings); err != nil {
			continue
		}

		// Check if Rakuten sync is enabled and has valid credentials
		enabled, _ := settings["rakuten_sync_enabled"].(bool)
		accessToken, _ := settings["rakuten_access_token"].(string)

		if enabled && accessToken != "" {
			enabledTenants = append(enabledTenants, row.ID)
		}
	}

	return enabledTenants, nil
}

// cleanupOldRakutenOrders removes old synced orders (older than 90 days)
func cleanupOldRakutenOrders() {
	log.Println("[Rakuten Cronjob] Starting old order cleanup...")

	cutoff := time.Now().AddDate(0, 0, -90)

	result := database.DB.Table("rakuten_orders").
		Where("created_at < ?", cutoff).
		Delete(nil)

	if result.Error != nil {
		log.Printf("[Rakuten Cronjob] Cleanup failed: %v", result.Error)
		return
	}

	log.Printf("[Rakuten Cronjob] Cleaned up %d old orders", result.RowsAffected)
}
