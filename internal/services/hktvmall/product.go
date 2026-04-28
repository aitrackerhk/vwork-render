package hktvmall

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Product represents a HKTVmall product
type Product struct {
	ProductID   string   `json:"product_id"`
	ProductCode string   `json:"product_code"` // SKU
	Name        string   `json:"name"`
	Description string   `json:"description"`
	CategoryID  string   `json:"category_id"`
	Brand       string   `json:"brand"`
	Images      []string `json:"images"`
	Status      string   `json:"status"` // active, inactive, pending
	CreateTime  int64    `json:"create_time"`
	UpdateTime  int64    `json:"update_time"`
}

// ProductVariant represents a product variant
type ProductVariant struct {
	VariantID string  `json:"variant_id"`
	ProductID string  `json:"product_id"`
	SKU       string  `json:"sku"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	OrigPrice float64 `json:"original_price"`
	Stock     int     `json:"stock"`
	Weight    float64 `json:"weight"`
	Status    string  `json:"status"`
}

// ProductListResponse represents the response from product list API
type ProductListResponse struct {
	Products    []Product `json:"products"`
	TotalCount  int       `json:"total_count"`
	CurrentPage int       `json:"current_page"`
	PageSize    int       `json:"page_size"`
	HasNextPage bool      `json:"has_next_page"`
}

// GetProductList retrieves the list of products
func (c *Client) GetProductList(page, pageSize int, status string) (*ProductListResponse, error) {
	params := map[string]string{
		"page":      strconv.Itoa(page),
		"page_size": strconv.Itoa(pageSize),
	}
	if status != "" {
		params["status"] = status
	}

	resp, err := c.Get("/api/v1/products", params)
	if err != nil {
		return nil, err
	}

	var result ProductListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse product list: %w", err)
	}

	return &result, nil
}

// GetProduct retrieves a single product by ID
func (c *Client) GetProduct(productID string) (*Product, error) {
	resp, err := c.Get("/api/v1/products/"+productID, nil)
	if err != nil {
		return nil, err
	}

	var product Product
	if err := json.Unmarshal(resp.Data, &product); err != nil {
		return nil, fmt.Errorf("failed to parse product: %w", err)
	}

	return &product, nil
}

// CreateProductRequest represents the request to create a product
type CreateProductRequest struct {
	ProductCode string   `json:"product_code"` // SKU
	Name        string   `json:"name"`
	Description string   `json:"description"`
	CategoryID  string   `json:"category_id"`
	Brand       string   `json:"brand,omitempty"`
	Images      []string `json:"images,omitempty"`
	Price       float64  `json:"price"`
	OrigPrice   float64  `json:"original_price,omitempty"`
	Stock       int      `json:"stock"`
	Weight      float64  `json:"weight,omitempty"`
}

// CreateProduct creates a new product
func (c *Client) CreateProduct(req *CreateProductRequest) (*Product, error) {
	resp, err := c.Post("/api/v1/products", req)
	if err != nil {
		return nil, err
	}

	var product Product
	if err := json.Unmarshal(resp.Data, &product); err != nil {
		return nil, fmt.Errorf("failed to parse created product: %w", err)
	}

	return &product, nil
}

// UpdateProductRequest represents the request to update a product
type UpdateProductRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	CategoryID  *string  `json:"category_id,omitempty"`
	Brand       *string  `json:"brand,omitempty"`
	Images      []string `json:"images,omitempty"`
	Status      *string  `json:"status,omitempty"`
}

// UpdateProduct updates an existing product
func (c *Client) UpdateProduct(productID string, req *UpdateProductRequest) error {
	_, err := c.Put("/api/v1/products/"+productID, req)
	return err
}

// UpdateStockRequest represents the request to update stock
type UpdateStockRequest struct {
	Stock int `json:"stock"`
}

// UpdateStock updates the stock of a product
func (c *Client) UpdateStock(productID string, stock int) error {
	req := UpdateStockRequest{Stock: stock}
	_, err := c.Put("/api/v1/products/"+productID+"/stock", req)
	return err
}

// UpdatePriceRequest represents the request to update price
type UpdatePriceRequest struct {
	Price     float64  `json:"price"`
	OrigPrice *float64 `json:"original_price,omitempty"`
}

// UpdatePrice updates the price of a product
func (c *Client) UpdatePrice(productID string, price float64, origPrice *float64) error {
	req := UpdatePriceRequest{Price: price, OrigPrice: origPrice}
	_, err := c.Put("/api/v1/products/"+productID+"/price", req)
	return err
}

// DeleteProduct deletes (or unlists) a product
func (c *Client) DeleteProduct(productID string) error {
	_, err := c.Delete("/api/v1/products/"+productID, nil)
	return err
}

// Category represents a product category
type Category struct {
	CategoryID string     `json:"category_id"`
	Name       string     `json:"name"`
	ParentID   string     `json:"parent_id,omitempty"`
	Level      int        `json:"level"`
	Children   []Category `json:"children,omitempty"`
}

// GetCategories retrieves the category tree
func (c *Client) GetCategories() ([]Category, error) {
	resp, err := c.Get("/api/v1/categories", nil)
	if err != nil {
		return nil, err
	}

	var categories []Category
	if err := json.Unmarshal(resp.Data, &categories); err != nil {
		return nil, fmt.Errorf("failed to parse categories: %w", err)
	}

	return categories, nil
}
