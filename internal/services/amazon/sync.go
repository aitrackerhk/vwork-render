package amazon

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"nwork/internal/database"

	"github.com/google/uuid"
)

// SyncService handles event-driven synchronization with Amazon
type SyncService struct{}

// NewSyncService creates a new Amazon sync service
func NewSyncService() *SyncService {
	return &SyncService{}
}

// Sync is the global sync service instance
var Sync = &SyncService{}

// AmazonSettings represents Amazon API settings for a tenant
type AmazonSettings struct {
	SellerID       string `json:"seller_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	RefreshToken   string `json:"refresh_token"`
	Region         string `json:"region"`          // "NA", "EU", "FE"
	MarketplaceID  string `json:"marketplace_id"`  // e.g., "ATVPDKIKX0DER" for US
	AWSAccessKeyID string `json:"aws_access_key_id"`
	AWSSecretKey   string `json:"aws_secret_key"`
	AWSRoleARN     string `json:"aws_role_arn"`
	Enabled        bool   `json:"enabled"`
	LastSyncTime   string `json:"last_sync_time"`
}

// ProductMapping represents a mapping between vWork product and Amazon listing
type ProductMapping struct {
	ProductID  uuid.UUID `json:"product_id"`
	SKU        string    `json:"sku"`  // Amazon Seller SKU
	ASIN       string    `json:"asin"` // Amazon ASIN
	Synced     bool      `json:"synced"`
	LastSyncAt string    `json:"last_sync_at"`
}

// GetAmazonSettings retrieves Amazon settings for a tenant
func (s *SyncService) GetAmazonSettings(tenantID uuid.UUID) (*AmazonSettings, error) {
	var extraFields json.RawMessage
	err := database.DB.Table("tenants").
		Select("extra_fields").
		Where("id = ?", tenantID).
		Scan(&extraFields).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant settings: %w", err)
	}

	if len(extraFields) == 0 {
		return nil, fmt.Errorf("amazon settings not found")
	}

	var fields map[string]interface{}
	if err := json.Unmarshal(extraFields, &fields); err != nil {
		return nil, fmt.Errorf("failed to parse extra_fields: %w", err)
	}

	amazonData, ok := fields["amazon"]
	if !ok {
		return nil, fmt.Errorf("amazon settings not found")
	}

	amazonJSON, err := json.Marshal(amazonData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal amazon settings: %w", err)
	}

	var settings AmazonSettings
	if err := json.Unmarshal(amazonJSON, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse amazon settings: %w", err)
	}

	return &settings, nil
}

// SaveAmazonSettings saves Amazon settings for a tenant
func (s *SyncService) SaveAmazonSettings(tenantID uuid.UUID, settings *AmazonSettings) error {
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

	fields["amazon"] = settings

	newExtraFields, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("failed to marshal extra_fields: %w", err)
	}

	return database.DB.Table("tenants").
		Where("id = ?", tenantID).
		Update("extra_fields", newExtraFields).Error
}

// GetAmazonClient creates an Amazon client for a tenant
func (s *SyncService) GetAmazonClient(tenantID uuid.UUID) (*Client, error) {
	settings, err := s.GetAmazonSettings(tenantID)
	if err != nil {
		return nil, err
	}

	if !settings.Enabled {
		return nil, fmt.Errorf("amazon integration is not enabled")
	}

	region := settings.Region
	if region == "" {
		region = "NA"
	}

	return NewClient(
		settings.ClientID,
		settings.ClientSecret,
		settings.RefreshToken,
		region,
		settings.MarketplaceID,
		settings.AWSAccessKeyID,
		settings.AWSSecretKey,
		settings.AWSRoleARN,
	), nil
}

// GetProductMapping retrieves Amazon SKU mapping for a vWork product
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
		return nil, fmt.Errorf("failed to parse extra_fields: %w", err)
	}

	amazonData, ok := fields["amazon"]
	if !ok {
		return nil, nil
	}

	amazonJSON, err := json.Marshal(amazonData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal amazon mapping: %w", err)
	}

	var mapping ProductMapping
	if err := json.Unmarshal(amazonJSON, &mapping); err != nil {
		return nil, fmt.Errorf("failed to parse amazon mapping: %w", err)
	}

	mapping.ProductID = productID
	return &mapping, nil
}

// SaveProductMapping saves Amazon SKU mapping for a vWork product
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

	fields["amazon"] = mapping

	newExtraFields, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("failed to marshal extra_fields: %w", err)
	}

	return database.DB.Table("products").
		Where("id = ? AND tenant_id = ?", productID, tenantID).
		Update("extra_fields", newExtraFields).Error
}

// SyncProductToAmazon syncs a product to Amazon (triggered on product create/update)
func (s *SyncService) SyncProductToAmazon(tenantID, productID uuid.UUID) {
	settings, err := s.GetAmazonSettings(tenantID)
	if err != nil || !settings.Enabled {
		return // Silently skip if not enabled
	}

	mapping, err := s.GetProductMapping(tenantID, productID)
	if err != nil || mapping == nil || mapping.SKU == "" {
		log.Printf("[Amazon Sync] Product %s has no Amazon SKU mapping, skipping", productID)
		return
	}

	client, err := s.GetAmazonClient(tenantID)
	if err != nil {
		log.Printf("[Amazon Sync] Failed to create client for tenant %s: %v", tenantID, err)
		return
	}

	// Get product data from vWork
	var product struct {
		ID          uuid.UUID `json:"id"`
		Name        string    `json:"name"`
		SKU         string    `json:"sku"`
		Price       float64   `json:"price"`
		Stock       int       `json:"stock"`
		Description string    `json:"description"`
	}
	if err := database.DB.Table("products").
		Where("id = ? AND tenant_id = ?", productID, tenantID).
		First(&product).Error; err != nil {
		log.Printf("[Amazon Sync] Failed to get product %s: %v", productID, err)
		return
	}

	// Update price on Amazon
	currency := s.getCurrencyForMarketplace(settings.MarketplaceID)
	if err := client.UpdatePrice(settings.SellerID, mapping.SKU, product.Price, currency); err != nil {
		log.Printf("[Amazon Sync] Failed to update price for SKU %s: %v", mapping.SKU, err)
	} else {
		log.Printf("[Amazon Sync] Updated price for SKU %s to %.2f %s", mapping.SKU, product.Price, currency)
	}

	// Update inventory on Amazon
	if err := client.UpdateInventory(settings.SellerID, mapping.SKU, product.Stock); err != nil {
		log.Printf("[Amazon Sync] Failed to update inventory for SKU %s: %v", mapping.SKU, err)
	} else {
		log.Printf("[Amazon Sync] Updated inventory for SKU %s to %d", mapping.SKU, product.Stock)
	}

	// Update mapping sync status
	mapping.Synced = true
	mapping.LastSyncAt = time.Now().Format(time.RFC3339)
	if err := s.SaveProductMapping(tenantID, productID, mapping); err != nil {
		log.Printf("[Amazon Sync] Failed to update mapping for product %s: %v", productID, err)
	}
}

// SyncInventoryToAmazon syncs inventory to Amazon (triggered on stock update)
func (s *SyncService) SyncInventoryToAmazon(tenantID, productID uuid.UUID, quantity int) {
	settings, err := s.GetAmazonSettings(tenantID)
	if err != nil || !settings.Enabled {
		return
	}

	mapping, err := s.GetProductMapping(tenantID, productID)
	if err != nil || mapping == nil || mapping.SKU == "" {
		return
	}

	client, err := s.GetAmazonClient(tenantID)
	if err != nil {
		log.Printf("[Amazon Sync] Failed to create client for tenant %s: %v", tenantID, err)
		return
	}

	if err := client.UpdateInventory(settings.SellerID, mapping.SKU, quantity); err != nil {
		log.Printf("[Amazon Sync] Failed to update inventory for SKU %s: %v", mapping.SKU, err)
	} else {
		log.Printf("[Amazon Sync] Updated inventory for SKU %s to %d", mapping.SKU, quantity)
	}
}

// SyncPriceToAmazon syncs price to Amazon (triggered on price update)
func (s *SyncService) SyncPriceToAmazon(tenantID, productID uuid.UUID, price float64) {
	settings, err := s.GetAmazonSettings(tenantID)
	if err != nil || !settings.Enabled {
		return
	}

	mapping, err := s.GetProductMapping(tenantID, productID)
	if err != nil || mapping == nil || mapping.SKU == "" {
		return
	}

	client, err := s.GetAmazonClient(tenantID)
	if err != nil {
		log.Printf("[Amazon Sync] Failed to create client for tenant %s: %v", tenantID, err)
		return
	}

	// Determine currency based on marketplace
	currency := s.getCurrencyForMarketplace(settings.MarketplaceID)

	if err := client.UpdatePrice(settings.SellerID, mapping.SKU, price, currency); err != nil {
		log.Printf("[Amazon Sync] Failed to update price for SKU %s: %v", mapping.SKU, err)
	} else {
		log.Printf("[Amazon Sync] Updated price for SKU %s to %.2f %s", mapping.SKU, price, currency)
	}
}

// SyncProductDeletionToAmazon handles product deletion (sets quantity to 0)
func (s *SyncService) SyncProductDeletionToAmazon(tenantID, productID uuid.UUID) {
	settings, err := s.GetAmazonSettings(tenantID)
	if err != nil || !settings.Enabled {
		return
	}

	mapping, err := s.GetProductMapping(tenantID, productID)
	if err != nil || mapping == nil || mapping.SKU == "" {
		return
	}

	client, err := s.GetAmazonClient(tenantID)
	if err != nil {
		log.Printf("[Amazon Sync] Failed to create client for tenant %s: %v", tenantID, err)
		return
	}

	// Set inventory to 0 to effectively "hide" the listing
	if err := client.UpdateInventory(settings.SellerID, mapping.SKU, 0); err != nil {
		log.Printf("[Amazon Sync] Failed to set inventory to 0 for deleted product SKU %s: %v", mapping.SKU, err)
	} else {
		log.Printf("[Amazon Sync] Set inventory to 0 for deleted product SKU %s", mapping.SKU)
	}
}

// getCurrencyForMarketplace returns the currency code for a marketplace
func (s *SyncService) getCurrencyForMarketplace(marketplaceID string) string {
	currencyMap := map[string]string{
		"ATVPDKIKX0DER":  "USD", // US
		"A2EUQ1WTGCTBG2": "CAD", // CA
		"A1AM78C64UM0Y8": "MXN", // MX
		"A2Q3Y263D00KWC": "BRL", // BR
		"A1RKKUPIHCS9HS": "EUR", // ES
		"A1PA6795UKMFR9": "EUR", // DE
		"A13V1IB3VIYBER": "EUR", // FR
		"APJ6JRA9NG5V4":  "EUR", // IT
		"A1F83G8C2ARO7P": "GBP", // UK
		"A21TJRUUN4KGV":  "INR", // IN
		"A1VC38T7YXB528": "JPY", // JP
		"A39IBJ37TRP1C6": "AUD", // AU
		"A19VAU5U5O7RUS": "SGD", // SG
		"A2VIGQ35RCS4UG": "AED", // AE
		"ARBP9OOSHTCHU":  "EGP", // EG
		"A33AVAJ2PDY3EV": "TRY", // TR
		"A17E79C6D8DWNP": "SAR", // SA
		"AAHKV2X7AFYLW":  "CNY", // CN
	}

	if currency, ok := currencyMap[marketplaceID]; ok {
		return currency
	}
	return "USD"
}

// GetAllEnabledTenants returns all tenants with Amazon integration enabled
func (s *SyncService) GetAllEnabledTenants() ([]uuid.UUID, error) {
	var tenants []struct {
		ID          uuid.UUID       `json:"id"`
		ExtraFields json.RawMessage `json:"extra_fields"`
	}

	if err := database.DB.Table("tenants").
		Select("id, extra_fields").
		Where("extra_fields IS NOT NULL").
		Find(&tenants).Error; err != nil {
		return nil, err
	}

	var enabledTenants []uuid.UUID
	for _, tenant := range tenants {
		var fields map[string]interface{}
		if err := json.Unmarshal(tenant.ExtraFields, &fields); err != nil {
			continue
		}

		amazonData, ok := fields["amazon"]
		if !ok {
			continue
		}

		amazonJSON, err := json.Marshal(amazonData)
		if err != nil {
			continue
		}

		var settings AmazonSettings
		if err := json.Unmarshal(amazonJSON, &settings); err != nil {
			continue
		}

		if settings.Enabled && settings.RefreshToken != "" {
			enabledTenants = append(enabledTenants, tenant.ID)
		}
	}

	return enabledTenants, nil
}
