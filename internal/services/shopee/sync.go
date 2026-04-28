package shopee

import (
	"log"
	"strconv"

	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/google/uuid"
)

// SyncService 提供事件驅動的 Shopee 同步功能
type SyncService struct{}

// NewSyncService 建立新的同步服務
func NewSyncService() *SyncService {
	return &SyncService{}
}

// SyncProductToShopee 當 vWork 產品更新時，同步到 Shopee
// 這個函數應該在產品 CRUD 操作後被調用
func (s *SyncService) SyncProductToShopee(tenantID uuid.UUID, productID uuid.UUID, action string) error {
	client, settings, err := s.getClientForTenant(tenantID)
	if err != nil {
		return nil // 沒有啟用 Shopee，靜默返回
	}

	syncProducts := parseBool(settings["sync_products"], true)
	if !syncProducts {
		return nil
	}

	log.Printf("🛒 Shopee sync triggered: tenant=%s, product=%s, action=%s", tenantID, productID, action)

	var product models.Product
	if err := database.DB.Where("id = ? AND tenant_id = ?", productID, tenantID).First(&product).Error; err != nil {
		log.Printf("❌ Product not found for sync: %s", productID)
		return nil
	}

	// 根據 action 執行不同操作
	switch action {
	case "create":
		return s.createProductOnShopee(client, &product)
	case "update":
		return s.updateProductOnShopee(client, &product)
	case "delete":
		return s.deleteProductOnShopee(client, &product)
	}

	return nil
}

func (s *SyncService) createProductOnShopee(client *Client, product *models.Product) error {
	// 檢查必要欄位
	// 注意：Shopee 需要 CategoryID, LogisticInfo 等，這裡假設存在 ExtraFields 或使用預設值
	// 實際專案中應該有更複雜的映射邏輯

	categoryID := int64(0)
	if v, ok := product.ExtraFields["shopee_category_id"].(float64); ok {
		categoryID = int64(v)
	}

	// 如果沒有分類ID，無法創建
	if categoryID == 0 {
		log.Printf("⚠️ Missing shopee_category_id for product %s, skipping create", product.ID)
		return nil
	}

	req := CreateProductRequest{
		OriginalPrice: product.Price,
		Description:   product.Description,
		ItemName:      product.Name,
		ItemStatus:    "NORMAL", // 預設上架
		NormalStock:   product.StockQuantity,
		Weight:        product.Weight,
		ItemSKU:       product.SKU,
		Condition:     "NEW",
		CategoryID:    categoryID,
	}

	// 處理圖片
	if product.ImageURL != "" {
		req.Image = ProductImage{
			ImageURLList: []string{product.ImageURL},
		}
	}

	// 處理物流 (必須至少有一個)
	// 這裡簡化處理，假設有一個預設物流ID存在 ExtraFields
	logisticID := int64(0)
	if v, ok := product.ExtraFields["shopee_logistic_id"].(float64); ok {
		logisticID = int64(v)
	}
	if logisticID > 0 {
		req.LogisticInfo = []LogisticInfo{
			{
				LogisticID: logisticID,
				Enabled:    true,
			},
		}
	}

	// 處理品牌 (Shopee 強制要求，若無品牌可填 No Brand 的 ID，通常是 0 或特定值)
	brandID := int64(0)
	if v, ok := product.ExtraFields["shopee_brand_id"].(float64); ok {
		brandID = int64(v)
	}
	req.Brand = BrandInfo{
		BrandID:           brandID,
		OriginalBrandName: "No Brand", // 簡化
	}

	newItem, err := client.CreateProduct(req)
	if err != nil {
		log.Printf("❌ Failed to create product on Shopee: %v", err)
		return err
	}

	log.Printf("✅ Created product on Shopee: vWorkID=%s, ShopeeID=%d", product.ID, newItem.ItemID)

	// 更新 vWork 產品的 shopee_item_id
	if product.ExtraFields == nil {
		product.ExtraFields = make(map[string]interface{})
	}
	product.ExtraFields["shopee_item_id"] = newItem.ItemID

	if err := database.DB.Model(product).Update("extra_fields", product.ExtraFields).Error; err != nil {
		log.Printf("⚠️ Failed to save shopee_item_id to database: %v", err)
	}

	return nil
}

func (s *SyncService) updateProductOnShopee(client *Client, product *models.Product) error {
	shopeeItemID := int64(0)
	if v, ok := product.ExtraFields["shopee_item_id"].(float64); ok {
		shopeeItemID = int64(v)
	}

	if shopeeItemID == 0 {
		log.Printf("⚠️ Product %s has no Shopee item_id, attempting create instead", product.ID)
		return s.createProductOnShopee(client, product)
	}

	req := UpdateProductRequest{
		ItemID:      shopeeItemID,
		Description: product.Description,
		ItemName:    product.Name,
		Weight:      product.Weight,
		ItemSKU:     product.SKU,
	}

	// 處理圖片更新
	if product.ImageURL != "" {
		req.Image = &ProductImage{
			ImageURLList: []string{product.ImageURL},
		}
	}

	if err := client.UpdateProduct(req); err != nil {
		log.Printf("❌ Failed to update product on Shopee: %v", err)
		return err
	}

	log.Printf("✅ Updated product on Shopee: %d", shopeeItemID)
	return nil
}

func (s *SyncService) deleteProductOnShopee(client *Client, product *models.Product) error {
	shopeeItemID := int64(0)
	if v, ok := product.ExtraFields["shopee_item_id"].(float64); ok {
		shopeeItemID = int64(v)
	}

	if shopeeItemID == 0 {
		log.Printf("⚠️ Product %s has no Shopee item_id, skipping delete", product.ID)
		return nil
	}

	if err := client.DeleteProduct(shopeeItemID); err != nil {
		log.Printf("❌ Failed to delete product on Shopee: %v", err)
		return err
	}

	log.Printf("✅ Deleted product on Shopee: %d", shopeeItemID)

	// 清除 shopee_item_id
	delete(product.ExtraFields, "shopee_item_id")
	if err := database.DB.Model(product).Update("extra_fields", product.ExtraFields).Error; err != nil {
		log.Printf("⚠️ Failed to remove shopee_item_id from database: %v", err)
	}

	return nil
}

// SyncInventoryToShopee 當 vWork 庫存更新時，同步到 Shopee

func (s *SyncService) SyncInventoryToShopee(tenantID uuid.UUID, productID uuid.UUID, stock int) error {
	client, settings, err := s.getClientForTenant(tenantID)
	if err != nil {
		return nil
	}

	syncInventory := parseBool(settings["sync_inventory"], true)
	if !syncInventory {
		return nil
	}

	// 需要查詢產品的 Shopee item_id（存在產品的 extra_fields 中）
	var product models.Product
	if err := database.DB.Where("id = ? AND tenant_id = ?", productID, tenantID).First(&product).Error; err != nil {
		return nil
	}

	shopeeItemID := int64(0)
	if product.ExtraFields != nil {
		if v, ok := product.ExtraFields["shopee_item_id"].(float64); ok {
			shopeeItemID = int64(v)
		}
	}

	if shopeeItemID == 0 {
		log.Printf("⚠️ Product %s has no Shopee item_id, skipping inventory sync", productID)
		return nil
	}

	stockList := []StockUpdateItem{
		{NormalStock: stock},
	}

	if err := client.UpdateStock(shopeeItemID, stockList); err != nil {
		log.Printf("❌ Failed to sync inventory to Shopee: %v", err)
		return err
	}

	log.Printf("✅ Synced inventory to Shopee: product=%s, item_id=%d, stock=%d", productID, shopeeItemID, stock)
	return nil
}

// SyncPriceToShopee 當 vWork 價格更新時，同步到 Shopee
func (s *SyncService) SyncPriceToShopee(tenantID uuid.UUID, productID uuid.UUID, price float64) error {
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

	shopeeItemID := int64(0)
	if product.ExtraFields != nil {
		if v, ok := product.ExtraFields["shopee_item_id"].(float64); ok {
			shopeeItemID = int64(v)
		}
	}

	if shopeeItemID == 0 {
		return nil
	}

	priceList := []PriceUpdateItem{
		{OriginalPrice: price},
	}

	if err := client.UpdatePrice(shopeeItemID, priceList); err != nil {
		log.Printf("❌ Failed to sync price to Shopee: %v", err)
		return err
	}

	log.Printf("✅ Synced price to Shopee: product=%s, item_id=%d, price=%.2f", productID, shopeeItemID, price)
	return nil
}

// getClientForTenant 取得租戶的 Shopee client
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

	shopeeSettings, ok := productSync["shopee"].(map[string]interface{})
	if !ok {
		return nil, nil, nil
	}

	enabled := parseBool(shopeeSettings["enabled"], false)
	if !enabled {
		return nil, nil, nil
	}

	accessToken := parseStr(shopeeSettings["access_token"], "")
	if accessToken == "" {
		return nil, nil, nil
	}

	appKeyStr := parseStr(shopeeSettings["app_key"], "")
	appSecret := parseStr(shopeeSettings["app_secret"], "")
	shopIDStr := parseStr(shopeeSettings["shop_id"], "")
	refreshToken := parseStr(shopeeSettings["refresh_token"], "")
	region := parseStr(shopeeSettings["region"], "TW")

	partnerID, _ := strconv.ParseInt(appKeyStr, 10, 64)
	shopID, _ := strconv.ParseInt(shopIDStr, 10, 64)

	client := NewClient(partnerID, appSecret, shopID, accessToken, refreshToken, region)
	return client, shopeeSettings, nil
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
