package hktvmall

import (
	"log"

	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/google/uuid"
)

// SyncService 提供事件驅動的 HKTVmall 同步功能
type SyncService struct{}

// NewSyncService 建立新的同步服務
func NewSyncService() *SyncService {
	return &SyncService{}
}

// SyncProductToHKTVmall 當 vWork 產品更新時，同步到 HKTVmall
// 這個函數應該在產品 CRUD 操作後被調用
func (s *SyncService) SyncProductToHKTVmall(tenantID uuid.UUID, productID uuid.UUID, action string) error {
	client, settings, err := s.getClientForTenant(tenantID)
	if err != nil {
		return nil // 沒有啟用 HKTVmall，靜默返回
	}

	syncProducts := parseBool(settings["sync_products"], true)
	if !syncProducts {
		return nil
	}

	log.Printf("🏪 HKTVmall sync triggered: tenant=%s, product=%s, action=%s", tenantID, productID, action)

	// 根據 action 執行不同操作
	switch action {
	case "create":
		return s.createProductOnHKTVmall(client, tenantID, productID)
	case "update":
		return s.updateProductOnHKTVmall(client, tenantID, productID)
	case "delete":
		return s.deleteProductOnHKTVmall(client, tenantID, productID)
	}

	return nil
}

// createProductOnHKTVmall 在 HKTVmall 創建商品
func (s *SyncService) createProductOnHKTVmall(client *Client, tenantID, productID uuid.UUID) error {
	var product models.Product
	if err := database.DB.Where("id = ? AND tenant_id = ?", productID, tenantID).First(&product).Error; err != nil {
		return nil
	}

	req := &CreateProductRequest{
		ProductCode: product.SKU,
		Name:        product.Name,
		Description: product.Description,
		Price:       product.Price,
		Stock:       product.StockQuantity,
	}

	created, err := client.CreateProduct(req)
	if err != nil {
		log.Printf("❌ Failed to create product on HKTVmall: %v", err)
		return err
	}

	// 保存 HKTVmall product_id 到 extra_fields
	if product.ExtraFields == nil {
		product.ExtraFields = make(map[string]interface{})
	}
	product.ExtraFields["hktvmall_product_id"] = created.ProductID

	if err := database.DB.Save(&product).Error; err != nil {
		log.Printf("⚠️ Failed to save HKTVmall product_id: %v", err)
	}

	log.Printf("✅ Created product on HKTVmall: vwork=%s, hktv=%s", productID, created.ProductID)
	return nil
}

// updateProductOnHKTVmall 在 HKTVmall 更新商品
func (s *SyncService) updateProductOnHKTVmall(client *Client, tenantID, productID uuid.UUID) error {
	var product models.Product
	if err := database.DB.Where("id = ? AND tenant_id = ?", productID, tenantID).First(&product).Error; err != nil {
		return nil
	}

	hktvProductID := s.getHKTVmallProductID(product)
	if hktvProductID == "" {
		log.Printf("⚠️ Product %s has no HKTVmall product_id, skipping update", productID)
		return nil
	}

	req := &UpdateProductRequest{
		Name:        &product.Name,
		Description: &product.Description,
	}

	if err := client.UpdateProduct(hktvProductID, req); err != nil {
		log.Printf("❌ Failed to update product on HKTVmall: %v", err)
		return err
	}

	log.Printf("✅ Updated product on HKTVmall: vwork=%s, hktv=%s", productID, hktvProductID)
	return nil
}

// deleteProductOnHKTVmall 在 HKTVmall 下架商品
func (s *SyncService) deleteProductOnHKTVmall(client *Client, tenantID, productID uuid.UUID) error {
	var product models.Product
	if err := database.DB.Where("id = ? AND tenant_id = ?", productID, tenantID).First(&product).Error; err != nil {
		return nil
	}

	hktvProductID := s.getHKTVmallProductID(product)
	if hktvProductID == "" {
		return nil
	}

	if err := client.DeleteProduct(hktvProductID); err != nil {
		log.Printf("❌ Failed to delete product on HKTVmall: %v", err)
		return err
	}

	log.Printf("✅ Deleted product on HKTVmall: vwork=%s, hktv=%s", productID, hktvProductID)
	return nil
}

// SyncInventoryToHKTVmall 當 vWork 庫存更新時，同步到 HKTVmall
func (s *SyncService) SyncInventoryToHKTVmall(tenantID uuid.UUID, productID uuid.UUID, stock int) error {
	client, settings, err := s.getClientForTenant(tenantID)
	if err != nil {
		return nil
	}

	syncInventory := parseBool(settings["sync_inventory"], true)
	if !syncInventory {
		return nil
	}

	var product models.Product
	if err := database.DB.Where("id = ? AND tenant_id = ?", productID, tenantID).First(&product).Error; err != nil {
		return nil
	}

	hktvProductID := s.getHKTVmallProductID(product)
	if hktvProductID == "" {
		log.Printf("⚠️ Product %s has no HKTVmall product_id, skipping inventory sync", productID)
		return nil
	}

	if err := client.UpdateStock(hktvProductID, stock); err != nil {
		log.Printf("❌ Failed to sync inventory to HKTVmall: %v", err)
		return err
	}

	log.Printf("✅ Synced inventory to HKTVmall: product=%s, hktv=%s, stock=%d", productID, hktvProductID, stock)
	return nil
}

// SyncPriceToHKTVmall 當 vWork 價格更新時，同步到 HKTVmall
func (s *SyncService) SyncPriceToHKTVmall(tenantID uuid.UUID, productID uuid.UUID, price float64) error {
	client, settings, err := s.getClientForTenant(tenantID)
	if err != nil {
		return nil
	}

	syncPrice := parseBool(settings["sync_price"], true)
	if !syncPrice {
		return nil
	}

	var product models.Product
	if err := database.DB.Where("id = ? AND tenant_id = ?", productID, tenantID).First(&product).Error; err != nil {
		return nil
	}

	hktvProductID := s.getHKTVmallProductID(product)
	if hktvProductID == "" {
		return nil
	}

	if err := client.UpdatePrice(hktvProductID, price, nil); err != nil {
		log.Printf("❌ Failed to sync price to HKTVmall: %v", err)
		return err
	}

	log.Printf("✅ Synced price to HKTVmall: product=%s, hktv=%s, price=%.2f", productID, hktvProductID, price)
	return nil
}

// getHKTVmallProductID 從產品的 extra_fields 中取得 HKTVmall product_id
func (s *SyncService) getHKTVmallProductID(product models.Product) string {
	if product.ExtraFields == nil {
		return ""
	}
	if v, ok := product.ExtraFields["hktvmall_product_id"].(string); ok {
		return v
	}
	return ""
}

// getClientForTenant 取得租戶的 HKTVmall client
func (s *SyncService) getClientForTenant(tenantID uuid.UUID) (*Client, map[string]interface{}, error) {
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil, nil, err
	}

	if tenant.ExtraFields == nil {
		return nil, nil, nil
	}

	productSync, ok := tenant.ExtraFields["product_sync"].(map[string]interface{})
	if !ok {
		return nil, nil, nil
	}

	hktvSettings, ok := productSync["hktvmall"].(map[string]interface{})
	if !ok {
		return nil, nil, nil
	}

	enabled := parseBool(hktvSettings["enabled"], false)
	if !enabled {
		return nil, nil, nil
	}

	merchantID := parseStr(hktvSettings["merchant_id"], "")
	shopID := parseStr(hktvSettings["shop_id"], "")
	appID := parseStr(hktvSettings["app_id"], "")
	appSecret := parseStr(hktvSettings["app_secret"], "")
	apiBase := parseStr(hktvSettings["api_base"], "")

	if appID == "" || appSecret == "" {
		return nil, nil, nil
	}

	client := NewClient(merchantID, shopID, appID, appSecret, apiBase)
	return client, hktvSettings, nil
}

// GetClientForTenant 公開版本，供外部調用
func GetClientForTenant(tenantID uuid.UUID) (*Client, map[string]interface{}, error) {
	return Sync.getClientForTenant(tenantID)
}

func parseStr(v interface{}, fallback string) string {
	if v == nil {
		return fallback
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fallback
}

func parseBool(v interface{}, fallback bool) bool {
	if v == nil {
		return fallback
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return fallback
}

// Global sync service instance
var Sync = NewSyncService()
