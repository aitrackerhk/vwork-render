package amazon

import (
	"encoding/json"
	"fmt"
)

// CatalogItem represents an Amazon catalog item
type CatalogItem struct {
	ASIN               string              `json:"asin"`
	Identifiers        []ItemIdentifier    `json:"identifiers"`
	Images             []ItemImage         `json:"images"`
	ProductTypes       []ProductType       `json:"productTypes"`
	SalesRanks         []SalesRank         `json:"salesRanks"`
	Summaries          []ItemSummary       `json:"summaries"`
	Variations         []ItemVariation     `json:"variations"`
	VendorDetails      []VendorDetail      `json:"vendorDetails"`
}

// ItemIdentifier represents product identifiers
type ItemIdentifier struct {
	MarketplaceID  string       `json:"marketplaceId"`
	Identifiers    []Identifier `json:"identifiers"`
}

// Identifier represents a single identifier
type Identifier struct {
	IdentifierType string `json:"identifierType"`
	Identifier     string `json:"identifier"`
}

// ItemImage represents product images
type ItemImage struct {
	MarketplaceID string  `json:"marketplaceId"`
	Images        []Image `json:"images"`
}

// Image represents a single image
type Image struct {
	Variant string `json:"variant"`
	Link    string `json:"link"`
	Height  int    `json:"height"`
	Width   int    `json:"width"`
}

// ProductType represents product type
type ProductType struct {
	MarketplaceID string `json:"marketplaceId"`
	ProductType   string `json:"productType"`
}

// SalesRank represents sales rankings
type SalesRank struct {
	MarketplaceID string `json:"marketplaceId"`
	ClassificationRanks []ClassificationRank `json:"classificationRanks"`
	DisplayGroupRanks   []DisplayGroupRank   `json:"displayGroupRanks"`
}

// ClassificationRank represents classification ranking
type ClassificationRank struct {
	ClassificationID string `json:"classificationId"`
	Title            string `json:"title"`
	Link             string `json:"link"`
	Rank             int    `json:"rank"`
}

// DisplayGroupRank represents display group ranking
type DisplayGroupRank struct {
	WebsiteDisplayGroup string `json:"websiteDisplayGroup"`
	Title               string `json:"title"`
	Link                string `json:"link"`
	Rank                int    `json:"rank"`
}

// ItemSummary represents item summary
type ItemSummary struct {
	MarketplaceID     string `json:"marketplaceId"`
	BrandName         string `json:"brandName"`
	BrowseNode        string `json:"browseNode"`
	ColorName         string `json:"colorName"`
	ItemName          string `json:"itemName"`
	Manufacturer      string `json:"manufacturer"`
	ModelNumber       string `json:"modelNumber"`
	SizeName          string `json:"sizeName"`
	StyleName         string `json:"styleName"`
}

// ItemVariation represents item variations
type ItemVariation struct {
	MarketplaceID string          `json:"marketplaceId"`
	ASINs         []string        `json:"asins"`
	VariationType string          `json:"variationType"`
}

// VendorDetail represents vendor details
type VendorDetail struct {
	MarketplaceID  string `json:"marketplaceId"`
	BrandCode      string `json:"brandCode"`
	CategoryCode   string `json:"categoryCode"`
	ManufacturerCode string `json:"manufacturerCode"`
	ManufacturerCodeParent string `json:"manufacturerCodeParent"`
	ProductGroup   string `json:"productGroup"`
	ReplenishmentCategory string `json:"replenishmentCategory"`
	SubcategoryCode string `json:"subcategoryCode"`
}

// ListingsItem represents a listings item
type ListingsItem struct {
	SKU                 string                 `json:"sku"`
	Summaries           []ListingSummary       `json:"summaries"`
	Attributes          map[string]interface{} `json:"attributes"`
	Issues              []Issue                `json:"issues"`
	Offers              []Offer                `json:"offers"`
	FulfillmentAvailability []FulfillmentAvailability `json:"fulfillmentAvailability"`
	Procurement         []Procurement          `json:"procurement"`
}

// ListingSummary represents listing summary
type ListingSummary struct {
	MarketplaceID   string   `json:"marketplaceId"`
	ASIN            string   `json:"asin"`
	ProductType     string   `json:"productType"`
	ConditionType   string   `json:"conditionType"`
	Status          []string `json:"status"`
	FNSku           string   `json:"fnSku"`
	ItemName        string   `json:"itemName"`
	CreatedDate     string   `json:"createdDate"`
	LastUpdatedDate string   `json:"lastUpdatedDate"`
	MainImage       *Image   `json:"mainImage"`
}

// Issue represents a listing issue
type Issue struct {
	Code         string   `json:"code"`
	Message      string   `json:"message"`
	Severity     string   `json:"severity"`
	AttributeNames []string `json:"attributeNames"`
}

// Offer represents a listing offer
type Offer struct {
	MarketplaceID string `json:"marketplaceId"`
	OfferType     string `json:"offerType"`
	Price         Money  `json:"price"`
}

// Money represents a monetary amount
type Money struct {
	CurrencyCode string  `json:"currencyCode"`
	Amount       float64 `json:"amount"`
}

// FulfillmentAvailability represents fulfillment availability
type FulfillmentAvailability struct {
	FulfillmentChannelCode string `json:"fulfillmentChannelCode"`
	Quantity               int    `json:"quantity"`
}

// Procurement represents procurement info
type Procurement struct {
	CostPrice Money `json:"costPrice"`
}

// GetCatalogItem retrieves a catalog item by ASIN
func (c *Client) GetCatalogItem(asin string, includedData []string) (*CatalogItem, error) {
	path := fmt.Sprintf("/catalog/2022-04-01/items/%s", asin)

	params := map[string]string{
		"marketplaceIds": c.MarketplaceID,
	}
	if len(includedData) > 0 {
		params["includedData"] = joinStrings(includedData, ",")
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, err
	}

	var item CatalogItem
	if err := json.Unmarshal(resp, &item); err != nil {
		return nil, fmt.Errorf("failed to parse catalog item: %w", err)
	}

	return &item, nil
}

// SearchCatalogItems searches for catalog items
func (c *Client) SearchCatalogItems(keywords string, includedData []string, pageSize int, pageToken string) ([]CatalogItem, string, error) {
	path := "/catalog/2022-04-01/items"

	params := map[string]string{
		"marketplaceIds": c.MarketplaceID,
		"keywords":       keywords,
		"pageSize":       fmt.Sprintf("%d", pageSize),
	}
	if len(includedData) > 0 {
		params["includedData"] = joinStrings(includedData, ",")
	}
	if pageToken != "" {
		params["pageToken"] = pageToken
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, "", err
	}

	var result struct {
		Items           []CatalogItem `json:"items"`
		NextPageToken   string        `json:"nextPageToken"`
		NumberOfResults int           `json:"numberOfResults"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse search results: %w", err)
	}

	return result.Items, result.NextPageToken, nil
}

// GetListingsItem retrieves a listings item
func (c *Client) GetListingsItem(sellerID, sku string, includedData []string) (*ListingsItem, error) {
	path := fmt.Sprintf("/listings/2021-08-01/items/%s/%s", sellerID, sku)

	params := map[string]string{
		"marketplaceIds": c.MarketplaceID,
	}
	if len(includedData) > 0 {
		params["includedData"] = joinStrings(includedData, ",")
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, err
	}

	var item ListingsItem
	if err := json.Unmarshal(resp, &item); err != nil {
		return nil, fmt.Errorf("failed to parse listings item: %w", err)
	}

	return &item, nil
}

// PatchListingsItem updates a listings item (for price/inventory updates)
func (c *Client) PatchListingsItem(sellerID, sku string, patches []ListingPatch) error {
	path := fmt.Sprintf("/listings/2021-08-01/items/%s/%s", sellerID, sku)

	params := map[string]string{
		"marketplaceIds": c.MarketplaceID,
	}

	body := map[string]interface{}{
		"productType": "PRODUCT", // Generic product type
		"patches":     patches,
	}

	_, err := c.Patch(path, params, body)
	return err
}

// ListingPatch represents a patch operation for listings
type ListingPatch struct {
	Op    string      `json:"op"`    // "add", "replace", "delete"
	Path  string      `json:"path"`  // JSON Pointer path
	Value interface{} `json:"value"` // Value to set
}

// UpdatePrice updates the price of a listing
func (c *Client) UpdatePrice(sellerID, sku string, price float64, currency string) error {
	patches := []ListingPatch{
		{
			Op:   "replace",
			Path: "/attributes/purchasable_offer",
			Value: []map[string]interface{}{
				{
					"marketplace_id": c.MarketplaceID,
					"currency":       currency,
					"our_price": []map[string]interface{}{
						{
							"schedule": []map[string]interface{}{
								{
									"value_with_tax": price,
								},
							},
						},
					},
				},
			},
		},
	}

	return c.PatchListingsItem(sellerID, sku, patches)
}

// UpdateInventory updates the inventory quantity of a listing
func (c *Client) UpdateInventory(sellerID, sku string, quantity int) error {
	patches := []ListingPatch{
		{
			Op:   "replace",
			Path: "/attributes/fulfillment_availability",
			Value: []map[string]interface{}{
				{
					"fulfillment_channel_code": "DEFAULT",
					"quantity":                 quantity,
				},
			},
		},
	}

	return c.PatchListingsItem(sellerID, sku, patches)
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
