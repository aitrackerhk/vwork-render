package lazada

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Product represents a Lazada product
type Product struct {
	ItemID          int64              `json:"item_id"`
	PrimaryCategory int64              `json:"primary_category"`
	Attributes      ProductAttributes  `json:"attributes"`
	Skus            []ProductSku       `json:"skus"`
	CreatedTime     string             `json:"created_time"`
	UpdatedTime     string             `json:"updated_time"`
	Status          string             `json:"status"`
}

// ProductAttributes represents product attributes
type ProductAttributes struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	ShortDescription string `json:"short_description"`
	Brand            string `json:"brand"`
	Model            string `json:"model"`
	Warranty         string `json:"warranty"`
	WarrantyType     string `json:"warranty_type"`
	Video            string `json:"video"`
}

// ProductSku represents a product SKU
type ProductSku struct {
	SkuID             int64              `json:"SkuId"`
	SellerSku         string             `json:"SellerSku"`
	ShopSku           string             `json:"ShopSku"`
	Url               string             `json:"Url"`
	Status            string             `json:"Status"`
	Quantity          int                `json:"quantity"`
	AvailableQuantity int                `json:"Available"`
	Price             float64            `json:"price"`
	SpecialPrice      float64            `json:"special_price"`
	SpecialFromDate   string             `json:"special_from_date"`
	SpecialToDate     string             `json:"special_to_date"`
	PackageWeight     string             `json:"package_weight"`
	PackageLength     string             `json:"package_length"`
	PackageWidth      string             `json:"package_width"`
	PackageHeight     string             `json:"package_height"`
	Images            []string           `json:"Images"`
	ColorFamily       string             `json:"color_family"`
	Size              string             `json:"size"`
	MultipleAttribute map[string]string  `json:"multipleAttribute"`
}

// ProductsResponse represents get products response
type ProductsResponse struct {
	TotalProducts int       `json:"total_products"`
	Products      []Product `json:"products"`
}

// GetProducts gets products with pagination
func (c *Client) GetProducts(filter string, offset, limit int) (*ProductsResponse, error) {
	params := map[string]string{
		"offset": strconv.Itoa(offset),
		"limit":  strconv.Itoa(limit),
	}
	if filter != "" {
		params["filter"] = filter
	}

	resp, err := c.Get("/products/get", params)
	if err != nil {
		return nil, err
	}

	var result ProductsResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse products: %w", err)
	}

	return &result, nil
}

// GetProduct gets a single product by item ID
func (c *Client) GetProduct(itemID int64) (*Product, error) {
	params := map[string]string{
		"item_id": strconv.FormatInt(itemID, 10),
	}

	resp, err := c.Get("/product/item/get", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data Product `json:"data"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		// Try parsing directly
		var product Product
		if err := json.Unmarshal(resp.Data, &product); err != nil {
			return nil, fmt.Errorf("failed to parse product: %w", err)
		}
		return &product, nil
	}

	return &result.Data, nil
}

// UpdatePrice updates product price
func (c *Client) UpdatePrice(skuID int64, sellerSku string, price float64) error {
	payload := `<Request>
		<Product>
			<Skus>
				<Sku>
					<SkuId>` + strconv.FormatInt(skuID, 10) + `</SkuId>
					<SellerSku>` + sellerSku + `</SellerSku>
					<Price>` + fmt.Sprintf("%.2f", price) + `</Price>
				</Sku>
			</Skus>
		</Product>
	</Request>`

	params := map[string]string{
		"payload": payload,
	}

	_, err := c.Post("/product/price_quantity/update", params, nil)
	return err
}

// UpdateStock updates product stock/quantity
func (c *Client) UpdateStock(skuID int64, sellerSku string, quantity int) error {
	payload := `<Request>
		<Product>
			<Skus>
				<Sku>
					<SkuId>` + strconv.FormatInt(skuID, 10) + `</SkuId>
					<SellerSku>` + sellerSku + `</SellerSku>
					<Quantity>` + strconv.Itoa(quantity) + `</Quantity>
				</Sku>
			</Skus>
		</Product>
	</Request>`

	params := map[string]string{
		"payload": payload,
	}

	_, err := c.Post("/product/price_quantity/update", params, nil)
	return err
}

// UpdatePriceAndStock updates both price and stock
func (c *Client) UpdatePriceAndStock(skuID int64, sellerSku string, price float64, quantity int) error {
	payload := `<Request>
		<Product>
			<Skus>
				<Sku>
					<SkuId>` + strconv.FormatInt(skuID, 10) + `</SkuId>
					<SellerSku>` + sellerSku + `</SellerSku>
					<Price>` + fmt.Sprintf("%.2f", price) + `</Price>
					<Quantity>` + strconv.Itoa(quantity) + `</Quantity>
				</Sku>
			</Skus>
		</Product>
	</Request>`

	params := map[string]string{
		"payload": payload,
	}

	_, err := c.Post("/product/price_quantity/update", params, nil)
	return err
}

// DeactivateProduct deactivates (removes) a product
func (c *Client) DeactivateProduct(itemID int64) error {
	params := map[string]string{
		"item_id": strconv.FormatInt(itemID, 10),
	}

	_, err := c.Post("/product/deactivate", params, nil)
	return err
}

// Category represents a Lazada category
type Category struct {
	CategoryID int64  `json:"category_id"`
	Name       string `json:"name"`
	Var        bool   `json:"var"`
	Leaf       bool   `json:"leaf"`
	Children   []Category `json:"children,omitempty"`
}

// GetCategories gets category tree
func (c *Client) GetCategories() ([]Category, error) {
	resp, err := c.Get("/category/tree/get", nil)
	if err != nil {
		return nil, err
	}

	var categories []Category
	if err := json.Unmarshal(resp.Data, &categories); err != nil {
		return nil, fmt.Errorf("failed to parse categories: %w", err)
	}

	return categories, nil
}

// CategoryAttribute represents a category attribute
type CategoryAttribute struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	InputType    string `json:"input_type"`
	IsMandatory  bool   `json:"is_mandatory"`
	IsSaleProp   bool   `json:"is_sale_prop"`
	AttributeType string `json:"attribute_type"`
	Options      []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"options,omitempty"`
}

// GetCategoryAttributes gets attributes for a category
func (c *Client) GetCategoryAttributes(categoryID int64) ([]CategoryAttribute, error) {
	params := map[string]string{
		"primary_category_id": strconv.FormatInt(categoryID, 10),
	}

	resp, err := c.Get("/category/attributes/get", params)
	if err != nil {
		return nil, err
	}

	var attrs []CategoryAttribute
	if err := json.Unmarshal(resp.Data, &attrs); err != nil {
		return nil, fmt.Errorf("failed to parse category attributes: %w", err)
	}

	return attrs, nil
}

// CreateProductRequest represents a product creation request
type CreateProductRequest struct {
	PrimaryCategory int64             `json:"primary_category"`
	SPUId           string            `json:"spu_id,omitempty"`
	AssociatedSku   string            `json:"associated_sku,omitempty"`
	Attributes      ProductAttributes `json:"attributes"`
	Skus            []CreateSkuData   `json:"skus"`
}

// CreateSkuData represents SKU data for product creation
type CreateSkuData struct {
	SellerSku     string   `json:"SellerSku"`
	Quantity      int      `json:"quantity"`
	Price         float64  `json:"price"`
	SpecialPrice  float64  `json:"special_price,omitempty"`
	PackageWeight string   `json:"package_weight"`
	PackageLength string   `json:"package_length"`
	PackageWidth  string   `json:"package_width"`
	PackageHeight string   `json:"package_height"`
	Images        []string `json:"images,omitempty"`
	ColorFamily   string   `json:"color_family,omitempty"`
	Size          string   `json:"size,omitempty"`
}

// CreateProduct creates a new product
func (c *Client) CreateProduct(req *CreateProductRequest) (int64, error) {
	// Convert to XML payload format required by Lazada
	skusXML := ""
	for _, sku := range req.Skus {
		imagesXML := ""
		for _, img := range sku.Images {
			imagesXML += "<Image>" + img + "</Image>"
		}
		
		skusXML += `<Sku>
			<SellerSku>` + sku.SellerSku + `</SellerSku>
			<quantity>` + strconv.Itoa(sku.Quantity) + `</quantity>
			<price>` + fmt.Sprintf("%.2f", sku.Price) + `</price>
			<package_weight>` + sku.PackageWeight + `</package_weight>
			<package_length>` + sku.PackageLength + `</package_length>
			<package_width>` + sku.PackageWidth + `</package_width>
			<package_height>` + sku.PackageHeight + `</package_height>
			<Images>` + imagesXML + `</Images>
		</Sku>`
	}

	payload := `<Request>
		<Product>
			<PrimaryCategory>` + strconv.FormatInt(req.PrimaryCategory, 10) + `</PrimaryCategory>
			<Attributes>
				<name>` + req.Attributes.Name + `</name>
				<description>` + req.Attributes.Description + `</description>
				<short_description>` + req.Attributes.ShortDescription + `</short_description>
				<brand>` + req.Attributes.Brand + `</brand>
			</Attributes>
			<Skus>` + skusXML + `</Skus>
		</Product>
	</Request>`

	params := map[string]string{
		"payload": payload,
	}

	resp, err := c.Post("/product/create", params, nil)
	if err != nil {
		return 0, err
	}

	var result struct {
		ItemID int64 `json:"item_id"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return 0, fmt.Errorf("failed to parse create response: %w", err)
	}

	return result.ItemID, nil
}
