package rakuten

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Item represents a Rakuten product item
type Item struct {
	ManageNumber    string           `json:"manageNumber"`    // 商品管理番号
	ItemNumber      string           `json:"itemNumber"`      // 商品番号
	ItemName        string           `json:"itemName"`        // 商品名
	ItemPrice       int              `json:"itemPrice"`       // 販売価格
	GenreID         int64            `json:"genreId"`         // ジャンルID
	CatalogID       string           `json:"catalogId"`       // カタログID
	ItemURL         string           `json:"itemUrl"`         // 商品URL
	TagLine         string           `json:"tagLine"`         // キャッチコピー
	Description     string           `json:"description"`     // PC用商品説明文
	MobileDesc      string           `json:"mobileDescription"` // モバイル用商品説明文
	SalesDesc       string           `json:"salesDescription"`  // PC用販売説明文
	Status          string           `json:"itemStatus"`       // 商品ステータス
	InventoryType   int              `json:"inventoryType"`   // 在庫タイプ
	TaxFlag         int              `json:"taxFlag"`         // 税区分
	PostageFlag     int              `json:"postageFlag"`     // 送料区分
	DeliverysetID   int              `json:"deliverysetId"`   // 配送方法セットID
	Images          []ItemImage      `json:"images"`          // 商品画像
	Variants        []ItemVariant    `json:"variants"`        // SKU/バリエーション
	CreatedTime     string           `json:"createdTime"`
	UpdatedTime     string           `json:"updatedTime"`
}

// ItemImage represents a product image
type ItemImage struct {
	ImageURL  string `json:"imageUrl"`
	ImageName string `json:"imageName"`
	ImageAlt  string `json:"imageAlt"`
}

// ItemVariant represents a product variant/SKU
type ItemVariant struct {
	VariantID       string  `json:"variantId"`
	SKU             string  `json:"sku"`
	JAN             string  `json:"jan"`
	Stock           int     `json:"stock"`
	Price           int     `json:"price"`
	PointRate       float64 `json:"pointRate"`
	ShippingWeight  float64 `json:"shippingWeight"`
	HorizontalName  string  `json:"horizontalName"`  // 横軸項目名
	HorizontalValue string  `json:"horizontalValue"` // 横軸選択肢
	VerticalName    string  `json:"verticalName"`    // 縦軸項目名
	VerticalValue   string  `json:"verticalValue"`   // 縦軸選択肢
}

// ItemsResponse represents get items response
type ItemsResponse struct {
	TotalCount int    `json:"totalCount"`
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
	Items      []Item `json:"items"`
}

// GetItems gets items with pagination
func (c *Client) GetItems(offset, limit int, status string) (*ItemsResponse, error) {
	params := map[string]string{
		"offset": strconv.Itoa(offset),
		"limit":  strconv.Itoa(limit),
	}
	if status != "" {
		params["itemStatus"] = status
	}

	resp, err := c.Get("/items/search", params)
	if err != nil {
		return nil, err
	}

	var result ItemsResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse items: %w", err)
	}

	return &result, nil
}

// GetItem gets a single item by manage number
func (c *Client) GetItem(manageNumber string) (*Item, error) {
	resp, err := c.Get("/items/"+manageNumber, nil)
	if err != nil {
		return nil, err
	}

	var item Item
	if err := json.Unmarshal(resp.Data, &item); err != nil {
		return nil, fmt.Errorf("failed to parse item: %w", err)
	}

	return &item, nil
}

// UpdateItemPrice updates item price
func (c *Client) UpdateItemPrice(manageNumber string, price int) error {
	body := map[string]interface{}{
		"itemPrice": price,
	}

	_, err := c.Patch("/items/"+manageNumber+"/price", body)
	return err
}

// UpdateItemStock updates item stock/inventory
func (c *Client) UpdateItemStock(manageNumber string, stock int) error {
	body := map[string]interface{}{
		"stock": stock,
	}

	_, err := c.Patch("/items/"+manageNumber+"/inventory", body)
	return err
}

// UpdateVariantStock updates variant stock
func (c *Client) UpdateVariantStock(manageNumber, variantID string, stock int) error {
	body := map[string]interface{}{
		"variantId": variantID,
		"stock":     stock,
	}

	_, err := c.Patch("/items/"+manageNumber+"/variants/inventory", body)
	return err
}

// UpdateItemPriceAndStock updates both price and stock
func (c *Client) UpdateItemPriceAndStock(manageNumber string, price, stock int) error {
	// Update price first
	if err := c.UpdateItemPrice(manageNumber, price); err != nil {
		return fmt.Errorf("failed to update price: %w", err)
	}

	// Then update stock
	if err := c.UpdateItemStock(manageNumber, stock); err != nil {
		return fmt.Errorf("failed to update stock: %w", err)
	}

	return nil
}

// DeactivateItem deactivates (sets to non-display) an item
func (c *Client) DeactivateItem(manageNumber string) error {
	body := map[string]interface{}{
		"itemStatus": "closed", // 非表示
	}

	_, err := c.Patch("/items/"+manageNumber+"/status", body)
	return err
}

// ActivateItem activates (sets to display) an item
func (c *Client) ActivateItem(manageNumber string) error {
	body := map[string]interface{}{
		"itemStatus": "normal", // 通常表示
	}

	_, err := c.Patch("/items/"+manageNumber+"/status", body)
	return err
}

// Genre represents a Rakuten category/genre
type Genre struct {
	GenreID   int64   `json:"genreId"`
	GenreName string  `json:"genreName"`
	Level     int     `json:"level"`
	ParentID  int64   `json:"parentId"`
	Children  []Genre `json:"children,omitempty"`
}

// GetGenres gets genre tree
func (c *Client) GetGenres(parentID int64) ([]Genre, error) {
	params := map[string]string{}
	if parentID > 0 {
		params["parentGenreId"] = strconv.FormatInt(parentID, 10)
	}

	resp, err := c.Get("/genres", params)
	if err != nil {
		return nil, err
	}

	var genres []Genre
	if err := json.Unmarshal(resp.Data, &genres); err != nil {
		return nil, fmt.Errorf("failed to parse genres: %w", err)
	}

	return genres, nil
}

// CreateItemRequest represents a request to create an item
type CreateItemRequest struct {
	ManageNumber  string        `json:"manageNumber"`
	ItemName      string        `json:"itemName"`
	ItemPrice     int           `json:"itemPrice"`
	GenreID       int64         `json:"genreId"`
	Description   string        `json:"description"`
	TagLine       string        `json:"tagLine,omitempty"`
	InventoryType int           `json:"inventoryType"`
	Stock         int           `json:"stock"`
	TaxFlag       int           `json:"taxFlag"`
	PostageFlag   int           `json:"postageFlag"`
	Images        []ItemImage   `json:"images,omitempty"`
	Variants      []ItemVariant `json:"variants,omitempty"`
}

// CreateItem creates a new item
func (c *Client) CreateItem(req *CreateItemRequest) (string, error) {
	resp, err := c.Post("/items", req)
	if err != nil {
		return "", err
	}

	var result struct {
		ManageNumber string `json:"manageNumber"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return "", fmt.Errorf("failed to parse create response: %w", err)
	}

	return result.ManageNumber, nil
}

// DeleteItem deletes an item
func (c *Client) DeleteItem(manageNumber string) error {
	_, err := c.Delete("/items/" + manageNumber)
	return err
}

// Inventory represents inventory information
type Inventory struct {
	ManageNumber string `json:"manageNumber"`
	ItemName     string `json:"itemName"`
	Stock        int    `json:"stock"`
	Variants     []struct {
		VariantID string `json:"variantId"`
		Stock     int    `json:"stock"`
	} `json:"variants,omitempty"`
}

// GetInventory gets inventory for an item
func (c *Client) GetInventory(manageNumber string) (*Inventory, error) {
	resp, err := c.Get("/inventory/"+manageNumber, nil)
	if err != nil {
		return nil, err
	}

	var inv Inventory
	if err := json.Unmarshal(resp.Data, &inv); err != nil {
		return nil, fmt.Errorf("failed to parse inventory: %w", err)
	}

	return &inv, nil
}

// BulkUpdateInventory bulk updates inventory
func (c *Client) BulkUpdateInventory(updates []map[string]interface{}) error {
	body := map[string]interface{}{
		"items": updates,
	}

	_, err := c.Post("/inventory/bulk", body)
	return err
}
