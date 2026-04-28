package lazada

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"
	"nwork/internal/database"
)

// SyncService handles Lazada sync operations
type SyncService struct {
	mu sync.RWMutex
}

// Sync is the global sync service instance
var Sync = &SyncService{}

// LazadaSettings represents Lazada connection settings for a tenant
type LazadaSettings struct {
	AppKey       string `json:"app_key"`
	AppSecret    string `json:"app_secret"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Region       string `json:"region"` // SG, MY, TH, PH, ID, VN
	SellerID     string `json:"seller_id"`
	ShopName     string `json:"shop_name"`
	Enabled      bool   `json:"enabled"`
	AutoSync     bool   `json:"auto_sync"`
	LastSyncAt   string `json:"last_sync_at"`
}

// ProductMapping represents the mapping between vWork product and Lazada item
type ProductMapping struct {
	ProductID    uuid.UUID `json:"product_id"`
	LazadaItemID int64     `json:"lazada_item_id"`
	LazadaSkuID  int64     `json:"lazada_sku_id"`
	SellerSku    string    `json:"seller_sku"`
	Synced       bool      `json:"synced"`
	LastSyncAt   string    `json:"last_sync_at"`
}

// GetLazadaSettings retrieves Lazada settings for a tenant
func (s *SyncService) GetLazadaSettings(tenantID uuid.UUID) (*LazadaSettings, error) {
	var extraFields json.RawMessage
	err := database.DB.Table("tenants").
		Select("extra_fields").
		Where("id = ?", tenantID).
		Scan(&extraFields).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant settings: %w", err)
	}

	if len(extraFields) == 0 {
		return nil, nil
	}

	var fields map[string]interface{}
	if err := json.Unmarshal(extraFields, &fields); err != nil {
		return nil, fmt.Errorf("failed to parse extra_fields: %w", err)
	}

	lazadaData, ok := fields["lazada_settings"]
	if !ok {
		return nil, nil
	}

	lazadaJSON, err := json.Marshal(lazadaData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal lazada settings: %w", err)
	}

	var settings LazadaSettings
	if err := json.Unmarshal(lazadaJSON, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse lazada settings: %w", err)
	}

	return &settings, nil
}

// SaveLazadaSettings saves Lazada settings for a tenant
func (s *SyncService) SaveLazadaSettings(tenantID uuid.UUID, settings *LazadaSettings) error {
	var extraFields json.RawMessage
	err := database.DB.Table("tenants").
		Select("extra_fields").
		Where("id = ?", tenantID).
		Scan(&extraFields).Error
	if err != nil {
		return fmt.Errorf("failed to get tenant settings: %w", err)
	}

	var fields map[string]interface{}
	if len(extraFields) > 0 {
		if err := json.Unmarshal(extraFields, &fields); err != nil {
			fields = make(map[string]interface{})
		}
	} else {
		fields = make(map[string]interface{})
	}

	fields["lazada_settings"] = settings

	newExtraFields, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("failed to marshal extra_fields: %w", err)
	}

	return database.DB.Table("tenants").
		Where("id = ?", tenantID).
		Update("extra_fields", newExtraFields).Error
}

// GetLazadaClient creates a Lazada client for a tenant
func (s *SyncService) GetLazadaClient(tenantID uuid.UUID) (*Client, error) {
	settings, err := s.GetLazadaSettings(tenantID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, fmt.Errorf("lazada not configured for tenant")
	}

	return NewClient(
		settings.AppKey,
		settings.AppSecret,
		settings.AccessToken,
		settings.RefreshToken,
		settings.Region,
	), nil
}

// GetProductMapping retrieves the Lazada mapping for a product
func (s *SyncService) GetProductMapping(tenantID, productID uuid.UUID) (*ProductMapping, error) {
	var extraFields json.RawMessage
	err := database.DB.Table("products").
		Select("extra_fields").
		Where("id = ? AND tenant_id = ?", productID, tenantID).
		Scan(&extraFields).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get product: %w", err)
	}

	if len(extraFields) == 0 {
		return nil, nil
	}

	var fields map[string]interface{}
	if err := json.Unmarshal(extraFields, &fields); err != nil {
		return nil, nil
	}

	mappingData, ok := fields["lazada_mapping"]
	if !ok {
		return nil, nil
	}

	mappingJSON, err := json.Marshal(mappingData)
	if err != nil {
		return nil, nil
	}

	var mapping ProductMapping
	if err := json.Unmarshal(mappingJSON, &mapping); err != nil {
		return nil, nil
	}

	mapping.ProductID = productID
	return &mapping, nil
}

// SaveProductMapping saves the Lazada mapping for a product
func (s *SyncService) SaveProductMapping(tenantID, productID uuid.UUID, mapping *ProductMapping) error {
	var extraFields json.RawMessage
	err := database.DB.Table("products").
		Select("extra_fields").
		Where("id = ? AND tenant_id = ?", productID, tenantID).
		Scan(&extraFields).Error
	if err != nil {
		return fmt.Errorf("failed to get product: %w", err)
	}

	var fields map[string]interface{}
	if len(extraFields) > 0 {
		if err := json.Unmarshal(extraFields, &fields); err != nil {
			fields = make(map[string]interface{})
		}
	} else {
		fields = make(map[string]interface{})
	}

	fields["lazada_mapping"] = mapping

	newExtraFields, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("failed to marshal extra_fields: %w", err)
	}

	return database.DB.Table("products").
		Where("id = ? AND tenant_id = ?", productID, tenantID).
		Update("extra_fields", newExtraFields).Error
}

// SyncProductToLazada syncs a product to Lazada (update price/stock)
func (s *SyncService) SyncProductToLazada(tenantID, productID uuid.UUID) error {
	settings, err := s.GetLazadaSettings(tenantID)
	if err != nil || settings == nil || !settings.Enabled {
		return nil // Skip if not enabled
	}

	mapping, err := s.GetProductMapping(tenantID, productID)
	if err != nil || mapping == nil {
		log.Printf("[Lazada] No mapping for product %s, skipping sync", productID)
		return nil
	}

	client, err := s.GetLazadaClient(tenantID)
	if err != nil {
		return fmt.Errorf("failed to get lazada client: %w", err)
	}

	// Get product details
	var product struct {
		ID    uuid.UUID `json:"id"`
		Name  string    `json:"name"`
		Price float64   `json:"price"`
	}
	if err := database.DB.Table("products").
		Where("id = ? AND tenant_id = ?", productID, tenantID).
		First(&product).Error; err != nil {
		return fmt.Errorf("product not found: %w", err)
	}

	// Get stock quantity from warehouse
	var totalStock int
	database.DB.Table("product_stocks").
		Select("COALESCE(SUM(quantity), 0)").
		Where("product_id = ?", productID).
		Scan(&totalStock)

	// Update price and stock on Lazada
	if err := client.UpdatePriceAndStock(
		mapping.LazadaSkuID,
		mapping.SellerSku,
		product.Price,
		totalStock,
	); err != nil {
		return fmt.Errorf("failed to update lazada: %w", err)
	}

	log.Printf("[Lazada] Synced product %s (Item: %d, SKU: %s) - Price: %.2f, Stock: %d",
		product.Name, mapping.LazadaItemID, mapping.SellerSku, product.Price, totalStock)

	return nil
}

// SyncInventoryToLazada syncs inventory to Lazada
func (s *SyncService) SyncInventoryToLazada(tenantID, productID uuid.UUID, quantity int) error {
	settings, err := s.GetLazadaSettings(tenantID)
	if err != nil || settings == nil || !settings.Enabled {
		return nil
	}

	mapping, err := s.GetProductMapping(tenantID, productID)
	if err != nil || mapping == nil {
		return nil
	}

	client, err := s.GetLazadaClient(tenantID)
	if err != nil {
		return err
	}

	if err := client.UpdateStock(mapping.LazadaSkuID, mapping.SellerSku, quantity); err != nil {
		return fmt.Errorf("failed to update lazada stock: %w", err)
	}

	log.Printf("[Lazada] Inventory synced for product %s: %d", productID, quantity)
	return nil
}

// SyncPriceToLazada syncs price to Lazada
func (s *SyncService) SyncPriceToLazada(tenantID, productID uuid.UUID, price float64) error {
	settings, err := s.GetLazadaSettings(tenantID)
	if err != nil || settings == nil || !settings.Enabled {
		return nil
	}

	mapping, err := s.GetProductMapping(tenantID, productID)
	if err != nil || mapping == nil {
		return nil
	}

	client, err := s.GetLazadaClient(tenantID)
	if err != nil {
		return err
	}

	if err := client.UpdatePrice(mapping.LazadaSkuID, mapping.SellerSku, price); err != nil {
		return fmt.Errorf("failed to update lazada price: %w", err)
	}

	log.Printf("[Lazada] Price synced for product %s: %.2f", productID, price)
	return nil
}

// SyncProductDeletionToLazada handles product deletion (deactivate on Lazada)
func (s *SyncService) SyncProductDeletionToLazada(tenantID, productID uuid.UUID) error {
	settings, err := s.GetLazadaSettings(tenantID)
	if err != nil || settings == nil || !settings.Enabled {
		return nil
	}

	mapping, err := s.GetProductMapping(tenantID, productID)
	if err != nil || mapping == nil {
		return nil
	}

	client, err := s.GetLazadaClient(tenantID)
	if err != nil {
		return err
	}

	if err := client.DeactivateProduct(mapping.LazadaItemID); err != nil {
		return fmt.Errorf("failed to deactivate lazada product: %w", err)
	}

	log.Printf("[Lazada] Product %s deactivated (Item: %d)", productID, mapping.LazadaItemID)
	return nil
}

// GetAllEnabledTenants returns all tenants with Lazada enabled
func (s *SyncService) GetAllEnabledTenants() ([]uuid.UUID, error) {
	var tenants []struct {
		ID          uuid.UUID       `json:"id"`
		ExtraFields json.RawMessage `json:"extra_fields"`
	}
	if err := database.DB.Table("tenants").
		Select("id, extra_fields").
		Find(&tenants).Error; err != nil {
		return nil, err
	}

	var enabledTenants []uuid.UUID
	for _, tenant := range tenants {
		settings, err := s.GetLazadaSettings(tenant.ID)
		if err != nil || settings == nil {
			continue
		}
		if settings.Enabled {
			enabledTenants = append(enabledTenants, tenant.ID)
		}
	}

	return enabledTenants, nil
}

// RefreshTokenForTenant refreshes the access token for a tenant
func (s *SyncService) RefreshTokenForTenant(tenantID uuid.UUID) error {
	settings, err := s.GetLazadaSettings(tenantID)
	if err != nil || settings == nil {
		return fmt.Errorf("lazada not configured")
	}

	client := NewClient(
		settings.AppKey,
		settings.AppSecret,
		settings.AccessToken,
		settings.RefreshToken,
		settings.Region,
	)

	tokenResp, err := client.RefreshAccessToken()
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	settings.AccessToken = tokenResp.AccessToken
	settings.RefreshToken = tokenResp.RefreshToken

	return s.SaveLazadaSettings(tenantID, settings)
}
