package rakuten

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"nwork/internal/database"
)

// SyncService handles synchronization between vWork and Rakuten
type SyncService struct {
	client   *Client
	tenantID uuid.UUID
}

// RakutenSettings represents Rakuten configuration stored in tenant
type RakutenSettings struct {
	ServiceSecret   string `json:"rakuten_service_secret"`
	LicenseKey      string `json:"rakuten_license_key"`
	ShopID          string `json:"rakuten_shop_id"`
	AccessToken     string `json:"rakuten_access_token"`
	RefreshToken    string `json:"rakuten_refresh_token"`
	TokenExpiry     int64  `json:"rakuten_token_expiry"`
	Enabled         bool   `json:"rakuten_sync_enabled"`
	LastOrderSync   int64  `json:"rakuten_last_order_sync"`
	LastProductSync int64  `json:"rakuten_last_product_sync"`
}

// ProductMapping represents mapping between vWork product and Rakuten item
type ProductMapping struct {
	ID              uuid.UUID `json:"id"`
	TenantID        uuid.UUID `json:"tenant_id"`
	ProductID       uuid.UUID `json:"product_id"`
	RakutenItemID   string    `json:"rakuten_item_id"`
	ManageNumber    string    `json:"manage_number"`
	SKU             string    `json:"sku"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// NewSyncService creates a new Rakuten sync service
func NewSyncService(tenantID uuid.UUID) (*SyncService, error) {
	settings, err := GetTenantSettings(tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant settings: %w", err)
	}

	client := NewClient(settings.ServiceSecret, settings.LicenseKey, settings.ShopID, settings.AccessToken, settings.RefreshToken)

	return &SyncService{
		client:   client,
		tenantID: tenantID,
	}, nil
}

// GetTenantSettings retrieves Rakuten settings from tenant
func GetTenantSettings(tenantID uuid.UUID) (*RakutenSettings, error) {
	var extraFields json.RawMessage
	err := database.DB.Table("tenants").
		Select("extra_fields").
		Where("id = ?", tenantID).
		Scan(&extraFields).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	var settings RakutenSettings
	if len(extraFields) > 0 {
		if err := json.Unmarshal(extraFields, &settings); err != nil {
			return nil, fmt.Errorf("failed to parse settings: %w", err)
		}
	}

	return &settings, nil
}

// SaveTenantSettings saves Rakuten settings to tenant
func SaveTenantSettings(tenantID uuid.UUID, settings *RakutenSettings) error {
	var currentFields json.RawMessage
	err := database.DB.Table("tenants").
		Select("extra_fields").
		Where("id = ?", tenantID).
		Scan(&currentFields).Error

	if err != nil {
		return fmt.Errorf("failed to get current settings: %w", err)
	}

	var fields map[string]interface{}
	if len(currentFields) > 0 {
		if err := json.Unmarshal(currentFields, &fields); err != nil {
			fields = make(map[string]interface{})
		}
	} else {
		fields = make(map[string]interface{})
	}

	// Update Rakuten fields
	fields["rakuten_service_secret"] = settings.ServiceSecret
	fields["rakuten_license_key"] = settings.LicenseKey
	fields["rakuten_shop_id"] = settings.ShopID
	fields["rakuten_access_token"] = settings.AccessToken
	fields["rakuten_refresh_token"] = settings.RefreshToken
	fields["rakuten_token_expiry"] = settings.TokenExpiry
	fields["rakuten_sync_enabled"] = settings.Enabled
	fields["rakuten_last_order_sync"] = settings.LastOrderSync
	fields["rakuten_last_product_sync"] = settings.LastProductSync

	return database.DB.Table("tenants").
		Where("id = ?", tenantID).
		Update("extra_fields", fields).Error
}

// GetProductMapping gets product mapping by vWork product ID
func (s *SyncService) GetProductMapping(productID uuid.UUID) (*ProductMapping, error) {
	var mapping ProductMapping
	err := database.DB.Table("rakuten_product_mappings").
		Where("tenant_id = ? AND product_id = ?", s.tenantID, productID).
		First(&mapping).Error

	if err != nil {
		return nil, err
	}

	return &mapping, nil
}

// GetProductMappingByManageNumber gets mapping by Rakuten manage number
func (s *SyncService) GetProductMappingByManageNumber(manageNumber string) (*ProductMapping, error) {
	var mapping ProductMapping
	err := database.DB.Table("rakuten_product_mappings").
		Where("tenant_id = ? AND manage_number = ?", s.tenantID, manageNumber).
		First(&mapping).Error

	if err != nil {
		return nil, err
	}

	return &mapping, nil
}

// CreateProductMapping creates a new product mapping
func (s *SyncService) CreateProductMapping(productID uuid.UUID, itemID, manageNumber, sku string) (*ProductMapping, error) {
	mapping := ProductMapping{
		ID:            uuid.New(),
		TenantID:      s.tenantID,
		ProductID:     productID,
		RakutenItemID: itemID,
		ManageNumber:  manageNumber,
		SKU:           sku,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := database.DB.Table("rakuten_product_mappings").Create(&mapping).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create mapping: %w", err)
	}

	return &mapping, nil
}

// SyncInventory syncs inventory from vWork to Rakuten
func (s *SyncService) SyncInventory(productID uuid.UUID, quantity int) error {
	mapping, err := s.GetProductMapping(productID)
	if err != nil {
		return fmt.Errorf("product not mapped to Rakuten: %w", err)
	}

	if err := s.client.UpdateItemStock(mapping.ManageNumber, quantity); err != nil {
		return fmt.Errorf("failed to update Rakuten stock: %w", err)
	}

	log.Printf("[Rakuten] Synced inventory for %s: %d units", mapping.ManageNumber, quantity)
	return nil
}

// SyncVariantInventory syncs variant inventory
func (s *SyncService) SyncVariantInventory(productID uuid.UUID, variantID string, quantity int) error {
	mapping, err := s.GetProductMapping(productID)
	if err != nil {
		return fmt.Errorf("product not mapped to Rakuten: %w", err)
	}

	if err := s.client.UpdateVariantStock(mapping.ManageNumber, variantID, quantity); err != nil {
		return fmt.Errorf("failed to update Rakuten variant stock: %w", err)
	}

	log.Printf("[Rakuten] Synced variant inventory for %s/%s: %d units", mapping.ManageNumber, variantID, quantity)
	return nil
}

// SyncPrice syncs price from vWork to Rakuten
func (s *SyncService) SyncPrice(productID uuid.UUID, price int) error {
	mapping, err := s.GetProductMapping(productID)
	if err != nil {
		return fmt.Errorf("product not mapped to Rakuten: %w", err)
	}

	if err := s.client.UpdateItemPrice(mapping.ManageNumber, price); err != nil {
		return fmt.Errorf("failed to update Rakuten price: %w", err)
	}

	log.Printf("[Rakuten] Synced price for %s: ¥%d", mapping.ManageNumber, price)
	return nil
}

// SyncPriceAndInventory syncs both price and inventory
func (s *SyncService) SyncPriceAndInventory(productID uuid.UUID, price, quantity int) error {
	mapping, err := s.GetProductMapping(productID)
	if err != nil {
		return fmt.Errorf("product not mapped to Rakuten: %w", err)
	}

	if err := s.client.UpdateItemPriceAndStock(mapping.ManageNumber, price, quantity); err != nil {
		return fmt.Errorf("failed to sync to Rakuten: %w", err)
	}

	log.Printf("[Rakuten] Synced %s: ¥%d, %d units", mapping.ManageNumber, price, quantity)
	return nil
}

// SyncOrders syncs orders from Rakuten to vWork
func (s *SyncService) SyncOrders() error {
	settings, err := GetTenantSettings(s.tenantID)
	if err != nil {
		return err
	}

	// Get orders since last sync (or last 24 hours)
	var startDate time.Time
	if settings.LastOrderSync > 0 {
		startDate = time.Unix(settings.LastOrderSync, 0)
	} else {
		startDate = time.Now().Add(-24 * time.Hour)
	}

	orders, err := s.client.GetOrders(startDate, time.Time{}, "", 0, 100)
	if err != nil {
		return fmt.Errorf("failed to get orders: %w", err)
	}

	for _, order := range orders.Orders {
		if err := s.processOrder(order); err != nil {
			log.Printf("[Rakuten] Failed to process order %s: %v", order.OrderNumber, err)
			continue
		}
	}

	// Update last sync time
	settings.LastOrderSync = time.Now().Unix()
	return SaveTenantSettings(s.tenantID, settings)
}

// processOrder processes a single Rakuten order
func (s *SyncService) processOrder(order Order) error {
	// Check if order exists
	var count int64
	database.DB.Table("rakuten_orders").
		Where("tenant_id = ? AND order_number = ?", s.tenantID, order.OrderNumber).
		Count(&count)

	orderData, _ := json.Marshal(order)

	if count == 0 {
		// Create new order
		return database.DB.Table("rakuten_orders").Create(map[string]interface{}{
			"id":           uuid.New(),
			"tenant_id":    s.tenantID,
			"order_number": order.OrderNumber,
			"order_status": order.OrderStatus,
			"total_price":  order.TotalPrice,
			"order_date":   order.OrderDate,
			"order_data":   orderData,
			"created_at":   time.Now(),
			"updated_at":   time.Now(),
		}).Error
	}

	// Update existing order
	return database.DB.Table("rakuten_orders").
		Where("tenant_id = ? AND order_number = ?", s.tenantID, order.OrderNumber).
		Updates(map[string]interface{}{
			"order_status": order.OrderStatus,
			"order_data":   orderData,
			"updated_at":   time.Now(),
		}).Error
}

// BulkSyncInventory syncs multiple products' inventory
func (s *SyncService) BulkSyncInventory(updates map[uuid.UUID]int) error {
	var inventoryUpdates []map[string]interface{}

	for productID, qty := range updates {
		mapping, err := s.GetProductMapping(productID)
		if err != nil {
			log.Printf("[Rakuten] Skipping unmapped product %s", productID)
			continue
		}

		inventoryUpdates = append(inventoryUpdates, map[string]interface{}{
			"manageNumber": mapping.ManageNumber,
			"stock":        qty,
		})
	}

	if len(inventoryUpdates) == 0 {
		return nil
	}

	return s.client.BulkUpdateInventory(inventoryUpdates)
}

// GetSyncStatus returns the current sync status
func (s *SyncService) GetSyncStatus() (map[string]interface{}, error) {
	settings, err := GetTenantSettings(s.tenantID)
	if err != nil {
		return nil, err
	}

	var mappingCount int64
	database.DB.Table("rakuten_product_mappings").
		Where("tenant_id = ?", s.tenantID).
		Count(&mappingCount)

	var orderCount int64
	database.DB.Table("rakuten_orders").
		Where("tenant_id = ?", s.tenantID).
		Count(&orderCount)

	return map[string]interface{}{
		"enabled":            settings.Enabled,
		"connected":          settings.AccessToken != "",
		"shop_id":            settings.ShopID,
		"last_order_sync":    time.Unix(settings.LastOrderSync, 0),
		"last_product_sync":  time.Unix(settings.LastProductSync, 0),
		"mapped_products":    mappingCount,
		"synced_orders":      orderCount,
		"token_expires":      time.Unix(settings.TokenExpiry, 0),
	}, nil
}

// DisconnectRakuten removes Rakuten connection
func DisconnectRakuten(tenantID uuid.UUID) error {
	settings := &RakutenSettings{
		Enabled: false,
	}
	return SaveTenantSettings(tenantID, settings)
}

// RefreshTokenIfNeeded refreshes the access token if it's about to expire
func (s *SyncService) RefreshTokenIfNeeded() error {
	settings, err := GetTenantSettings(s.tenantID)
	if err != nil {
		return err
	}

	// Check if token expires in less than 1 hour
	if time.Unix(settings.TokenExpiry, 0).Before(time.Now().Add(time.Hour)) {
		tokenResp, err := s.client.RefreshAccessToken()
		if err != nil {
			return fmt.Errorf("failed to refresh token: %w", err)
		}

		// Save new tokens
		settings.AccessToken = tokenResp.AccessToken
		settings.RefreshToken = tokenResp.RefreshToken
		settings.TokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Unix()

		return SaveTenantSettings(s.tenantID, settings)
	}

	return nil
}

// TestConnection tests the Rakuten API connection
func (s *SyncService) TestConnection() error {
	_, err := s.client.GetShopInfo()
	return err
}

// GetClient returns the underlying Rakuten client
func (s *SyncService) GetClient() *Client {
	return s.client
}
