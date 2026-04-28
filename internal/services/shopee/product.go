package shopee

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Product represents a Shopee product
type Product struct {
	ItemID          int64          `json:"item_id"`
	CategoryID      int64          `json:"category_id"`
	ItemName        string         `json:"item_name"`
	Description     string         `json:"description"`
	ItemSKU         string         `json:"item_sku"`
	CreateTime      int64          `json:"create_time"`
	UpdateTime      int64          `json:"update_time"`
	Weight          float64        `json:"weight"`
	Image           ProductImage   `json:"image"`
	PreOrder        PreOrderInfo   `json:"pre_order"`
	Dimension       Dimension      `json:"dimension"`
	Logistic        []LogisticInfo `json:"logistic_info"`
	Condition       string         `json:"condition"`
	SizeChart       string         `json:"size_chart"`
	ItemStatus      string         `json:"item_status"`
	HasModel        bool           `json:"has_model"`
	PromotionID     int64          `json:"promotion_id"`
	Brand           BrandInfo      `json:"brand"`
	ItemDangerous   int            `json:"item_dangerous"`
	ComplaintPolicy string         `json:"complaint_policy"`
	StockInfo       []StockInfo    `json:"stock_info"`
	PriceInfo       []PriceInfo    `json:"price_info"`
}

// ProductImage represents product images
type ProductImage struct {
	ImageIDList  []string `json:"image_id_list"`
	ImageURLList []string `json:"image_url_list"`
}

// PreOrderInfo represents pre-order information
type PreOrderInfo struct {
	IsPreOrder bool `json:"is_pre_order"`
	DaysToShip int  `json:"days_to_ship"`
}

// Dimension represents product dimensions
type Dimension struct {
	PackageLength int `json:"package_length"`
	PackageWidth  int `json:"package_width"`
	PackageHeight int `json:"package_height"`
}

// LogisticInfo represents logistics information
type LogisticInfo struct {
	LogisticID        int64   `json:"logistic_id"`
	LogisticName      string  `json:"logistic_name"`
	Enabled           bool    `json:"enabled"`
	ShippingFee       float64 `json:"shipping_fee"`
	IsFree            bool    `json:"is_free"`
	EstimatedShipping string  `json:"estimated_shipping_fee"`
}

// BrandInfo represents brand information
type BrandInfo struct {
	BrandID           int64  `json:"brand_id"`
	OriginalBrandName string `json:"original_brand_name"`
}

// Model represents a product variation/model
type Model struct {
	ModelID     int64        `json:"model_id"`
	TierIndex   []int        `json:"tier_index"`
	StockInfo   []StockInfo  `json:"stock_info"`
	PriceInfo   []PriceInfo  `json:"price_info"`
	ModelSKU    string       `json:"model_sku"`
	PreOrder    PreOrderInfo `json:"pre_order"`
	ModelStatus string       `json:"model_status"`
}

// StockInfo represents stock information
type StockInfo struct {
	StockType       int    `json:"stock_type"`
	StockLocationID string `json:"stock_location_id"`
	CurrentStock    int    `json:"current_stock"`
	NormalStock     int    `json:"normal_stock"`
	ReservedStock   int    `json:"reserved_stock"`
}

// PriceInfo represents price information
type PriceInfo struct {
	Currency                     string  `json:"currency"`
	OriginalPrice                float64 `json:"original_price"`
	CurrentPrice                 float64 `json:"current_price"`
	InflatedPriceOfOriginalPrice float64 `json:"inflated_price_of_original_price"`
	InflatedPriceOfCurrentPrice  float64 `json:"inflated_price_of_current_price"`
	SIPItemPrice                 float64 `json:"sip_item_price"`
	SIPItemPriceSource           string  `json:"sip_item_price_source"`
}

// CreateProductRequest represents the request to create a product
type CreateProductRequest struct {
	OriginalPrice float64        `json:"original_price"`
	Description   string         `json:"description"`
	ItemName      string         `json:"item_name"`
	ItemStatus    string         `json:"item_status"`
	NormalStock   int            `json:"normal_stock"`
	LogisticInfo  []LogisticInfo `json:"logistic_info"`
	Weight        float64        `json:"weight"`
	ItemSKU       string         `json:"item_sku"`
	Condition     string         `json:"condition"`
	CategoryID    int64          `json:"category_id"`
	Brand         BrandInfo      `json:"brand"`
	Image         ProductImage   `json:"image"`
}

// CreateProduct creates a new product on Shopee
func (c *Client) CreateProduct(req CreateProductRequest) (*ItemBasic, error) {
	path := "/api/v2/product/add_item"
	resp, err := c.Post(path, nil, req)
	if err != nil {
		return nil, err
	}

	var result struct {
		ItemID int64 `json:"item_id"`
	}
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("failed to parse create product response: %w", err)
	}

	return &ItemBasic{ItemID: result.ItemID}, nil
}

// UpdateProductRequest represents the request to update a product
type UpdateProductRequest struct {
	ItemID      int64         `json:"item_id"`
	Description string        `json:"description,omitempty"`
	ItemName    string        `json:"item_name,omitempty"`
	ItemStatus  string        `json:"item_status,omitempty"`
	Weight      float64       `json:"weight,omitempty"`
	ItemSKU     string        `json:"item_sku,omitempty"`
	Condition   string        `json:"condition,omitempty"`
	CategoryID  int64         `json:"category_id,omitempty"`
	Brand       *BrandInfo    `json:"brand,omitempty"`
	Image       *ProductImage `json:"image,omitempty"`
}

// UpdateProduct updates an existing product on Shopee
func (c *Client) UpdateProduct(req UpdateProductRequest) error {
	path := "/api/v2/product/update_item"
	_, err := c.Post(path, nil, req)
	return err
}

// DeleteProduct deletes a product on Shopee
func (c *Client) DeleteProduct(itemID int64) error {
	path := "/api/v2/product/delete_item"
	body := map[string]interface{}{
		"item_id": itemID,
	}
	_, err := c.Post(path, nil, body)
	return err
}

// GetItemListResponse represents the response from get_item_list API
type GetItemListResponse struct {
	Items       []ItemBasic `json:"item"`
	TotalCount  int         `json:"total_count"`
	HasNextPage bool        `json:"has_next_page"`
	NextOffset  string      `json:"next_offset"`
}

// ItemBasic represents basic item info returned from list API
type ItemBasic struct {
	ItemID     int64  `json:"item_id"`
	ItemStatus string `json:"item_status"`
	UpdateTime int64  `json:"update_time"`
}

// GetItemList retrieves the list of items
func (c *Client) GetItemList(offset int, pageSize int, itemStatus string) (*GetItemListResponse, error) {
	path := "/api/v2/product/get_item_list"

	params := map[string]string{
		"offset":      strconv.Itoa(offset),
		"page_size":   strconv.Itoa(pageSize),
		"item_status": itemStatus,
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, err
	}

	var result GetItemListResponse
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("failed to parse item list: %w", err)
	}

	return &result, nil
}

// GetItemBaseInfo retrieves base info for items
func (c *Client) GetItemBaseInfo(itemIDs []int64) ([]Product, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	if len(itemIDs) > 50 {
		return nil, fmt.Errorf("maximum 50 items per request")
	}

	path := "/api/v2/product/get_item_base_info"

	// Convert item IDs to comma-separated string
	itemIDsStr := ""
	for i, id := range itemIDs {
		if i > 0 {
			itemIDsStr += ","
		}
		itemIDsStr += strconv.FormatInt(id, 10)
	}

	params := map[string]string{
		"item_id_list": itemIDsStr,
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, err
	}

	var result struct {
		ItemList []Product `json:"item_list"`
	}
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("failed to parse item base info: %w", err)
	}

	return result.ItemList, nil
}

// GetModelList retrieves models (variations) for an item
func (c *Client) GetModelList(itemID int64) ([]Model, error) {
	path := "/api/v2/product/get_model_list"

	params := map[string]string{
		"item_id": strconv.FormatInt(itemID, 10),
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, err
	}

	var result struct {
		TierVariation []interface{} `json:"tier_variation"`
		Model         []Model       `json:"model"`
	}
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("failed to parse model list: %w", err)
	}

	return result.Model, nil
}

// UpdateStockRequest represents the request body for updating stock
type UpdateStockRequest struct {
	ItemID    int64             `json:"item_id"`
	StockList []StockUpdateItem `json:"stock_list"`
}

// StockUpdateItem represents a single stock update
type StockUpdateItem struct {
	ModelID     int64 `json:"model_id,omitempty"`
	NormalStock int   `json:"normal_stock"`
}

// UpdateStock updates stock for an item
func (c *Client) UpdateStock(itemID int64, stockList []StockUpdateItem) error {
	path := "/api/v2/product/update_stock"

	body := UpdateStockRequest{
		ItemID:    itemID,
		StockList: stockList,
	}

	_, err := c.Post(path, nil, body)
	return err
}

// UpdatePriceRequest represents the request body for updating price
type UpdatePriceRequest struct {
	ItemID    int64             `json:"item_id"`
	PriceList []PriceUpdateItem `json:"price_list"`
}

// PriceUpdateItem represents a single price update
type PriceUpdateItem struct {
	ModelID       int64   `json:"model_id,omitempty"`
	OriginalPrice float64 `json:"original_price"`
}

// UpdatePrice updates price for an item
func (c *Client) UpdatePrice(itemID int64, priceList []PriceUpdateItem) error {
	path := "/api/v2/product/update_price"

	body := UpdatePriceRequest{
		ItemID:    itemID,
		PriceList: priceList,
	}

	_, err := c.Post(path, nil, body)
	return err
}

// GetAllProducts retrieves all products with pagination
func (c *Client) GetAllProducts(itemStatus string) ([]Product, error) {
	var allProducts []Product
	offset := 0
	pageSize := 50

	for {
		listResp, err := c.GetItemList(offset, pageSize, itemStatus)
		if err != nil {
			return nil, err
		}

		if len(listResp.Items) == 0 {
			break
		}

		// Get item IDs
		itemIDs := make([]int64, len(listResp.Items))
		for i, item := range listResp.Items {
			itemIDs[i] = item.ItemID
		}

		// Get base info for items
		products, err := c.GetItemBaseInfo(itemIDs)
		if err != nil {
			return nil, err
		}

		allProducts = append(allProducts, products...)

		if !listResp.HasNextPage {
			break
		}

		offset += pageSize
	}

	return allProducts, nil
}
