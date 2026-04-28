package amazon

import (
	"encoding/json"
	"fmt"
	"time"
)

// Order represents an Amazon order
type Order struct {
	AmazonOrderID                  string          `json:"AmazonOrderId"`
	SellerOrderID                  string          `json:"SellerOrderId"`
	PurchaseDate                   string          `json:"PurchaseDate"`
	LastUpdateDate                 string          `json:"LastUpdateDate"`
	OrderStatus                    string          `json:"OrderStatus"`
	FulfillmentChannel             string          `json:"FulfillmentChannel"`
	SalesChannel                   string          `json:"SalesChannel"`
	OrderChannel                   string          `json:"OrderChannel"`
	ShipServiceLevel               string          `json:"ShipServiceLevel"`
	OrderTotal                     *OrderMoney     `json:"OrderTotal"`
	NumberOfItemsShipped           int             `json:"NumberOfItemsShipped"`
	NumberOfItemsUnshipped         int             `json:"NumberOfItemsUnshipped"`
	PaymentExecutionDetail         []PaymentDetail `json:"PaymentExecutionDetail"`
	PaymentMethod                  string          `json:"PaymentMethod"`
	PaymentMethodDetails           []string        `json:"PaymentMethodDetails"`
	MarketplaceID                  string          `json:"MarketplaceId"`
	ShipmentServiceLevelCategory   string          `json:"ShipmentServiceLevelCategory"`
	EasyShipShipmentStatus         string          `json:"EasyShipShipmentStatus"`
	OrderType                      string          `json:"OrderType"`
	EarliestShipDate               string          `json:"EarliestShipDate"`
	LatestShipDate                 string          `json:"LatestShipDate"`
	EarliestDeliveryDate           string          `json:"EarliestDeliveryDate"`
	LatestDeliveryDate             string          `json:"LatestDeliveryDate"`
	IsBusinessOrder                bool            `json:"IsBusinessOrder"`
	IsPrime                        bool            `json:"IsPrime"`
	IsPremiumOrder                 bool            `json:"IsPremiumOrder"`
	IsGlobalExpressEnabled         bool            `json:"IsGlobalExpressEnabled"`
	ReplacedOrderID                string          `json:"ReplacedOrderId"`
	IsReplacementOrder             bool            `json:"IsReplacementOrder"`
	PromiseResponseDueDate         string          `json:"PromiseResponseDueDate"`
	IsEstimatedShipDateSet         bool            `json:"IsEstimatedShipDateSet"`
	IsSoldByAB                     bool            `json:"IsSoldByAB"`
	IsIBA                          bool            `json:"IsIBA"`
	DefaultShipFromLocationAddress *Address        `json:"DefaultShipFromLocationAddress"`
	BuyerInvoicePreference         string          `json:"BuyerInvoicePreference"`
	BuyerTaxInformation            *TaxInformation `json:"BuyerTaxInformation"`
	FulfillmentInstruction         *FulfillmentInstruction `json:"FulfillmentInstruction"`
	IsISPU                         bool            `json:"IsISPU"`
	IsAccessPointOrder             bool            `json:"IsAccessPointOrder"`
	MarketplaceTaxInfo             *MarketplaceTaxInfo `json:"MarketplaceTaxInfo"`
	SellerDisplayName              string          `json:"SellerDisplayName"`
	ShippingAddress                *Address        `json:"ShippingAddress"`
	BuyerInfo                      *BuyerInfo      `json:"BuyerInfo"`
}

// OrderMoney represents monetary value in orders
type OrderMoney struct {
	CurrencyCode string `json:"CurrencyCode"`
	Amount       string `json:"Amount"`
}

// PaymentDetail represents payment execution detail
type PaymentDetail struct {
	PaymentMethod string      `json:"PaymentMethod"`
	PaymentAmount *OrderMoney `json:"PaymentAmount"`
}

// Address represents a shipping/billing address
type Address struct {
	Name                     string `json:"Name"`
	AddressLine1             string `json:"AddressLine1"`
	AddressLine2             string `json:"AddressLine2"`
	AddressLine3             string `json:"AddressLine3"`
	City                     string `json:"City"`
	County                   string `json:"County"`
	District                 string `json:"District"`
	StateOrRegion            string `json:"StateOrRegion"`
	Municipality             string `json:"Municipality"`
	PostalCode               string `json:"PostalCode"`
	CountryCode              string `json:"CountryCode"`
	Phone                    string `json:"Phone"`
	AddressType              string `json:"AddressType"`
}

// TaxInformation represents buyer tax information
type TaxInformation struct {
	TaxClassifications []TaxClassification `json:"TaxClassifications"`
}

// TaxClassification represents tax classification
type TaxClassification struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// FulfillmentInstruction represents fulfillment instruction
type FulfillmentInstruction struct {
	FulfillmentSupplySourceID string `json:"FulfillmentSupplySourceId"`
}

// MarketplaceTaxInfo represents marketplace tax info
type MarketplaceTaxInfo struct {
	TaxClassifications []TaxClassification `json:"TaxClassifications"`
}

// BuyerInfo represents buyer information
type BuyerInfo struct {
	BuyerEmail    string `json:"BuyerEmail"`
	BuyerName     string `json:"BuyerName"`
	BuyerCounty   string `json:"BuyerCounty"`
	BuyerTaxInfo  *TaxInformation `json:"BuyerTaxInfo"`
	PurchaseOrderNumber string `json:"PurchaseOrderNumber"`
}

// OrderItem represents an item in an order
type OrderItem struct {
	ASIN                          string      `json:"ASIN"`
	SellerSKU                     string      `json:"SellerSKU"`
	OrderItemID                   string      `json:"OrderItemId"`
	Title                         string      `json:"Title"`
	QuantityOrdered               int         `json:"QuantityOrdered"`
	QuantityShipped               int         `json:"QuantityShipped"`
	ProductInfo                   *ProductInfo `json:"ProductInfo"`
	PointsGranted                 *PointsGranted `json:"PointsGranted"`
	ItemPrice                     *OrderMoney `json:"ItemPrice"`
	ShippingPrice                 *OrderMoney `json:"ShippingPrice"`
	ItemTax                       *OrderMoney `json:"ItemTax"`
	ShippingTax                   *OrderMoney `json:"ShippingTax"`
	ShippingDiscount              *OrderMoney `json:"ShippingDiscount"`
	ShippingDiscountTax           *OrderMoney `json:"ShippingDiscountTax"`
	PromotionDiscount             *OrderMoney `json:"PromotionDiscount"`
	PromotionDiscountTax          *OrderMoney `json:"PromotionDiscountTax"`
	PromotionIDs                  []string    `json:"PromotionIds"`
	CODFee                        *OrderMoney `json:"CODFee"`
	CODFeeDiscount                *OrderMoney `json:"CODFeeDiscount"`
	IsGift                        bool        `json:"IsGift"`
	ConditionNote                 string      `json:"ConditionNote"`
	ConditionID                   string      `json:"ConditionId"`
	ConditionSubtypeID            string      `json:"ConditionSubtypeId"`
	ScheduledDeliveryStartDate    string      `json:"ScheduledDeliveryStartDate"`
	ScheduledDeliveryEndDate      string      `json:"ScheduledDeliveryEndDate"`
	PriceDesignation              string      `json:"PriceDesignation"`
	TaxCollection                 *TaxCollection `json:"TaxCollection"`
	SerialNumberRequired          bool        `json:"SerialNumberRequired"`
	IsTransparency                bool        `json:"IsTransparency"`
	IossNumber                    string      `json:"IossNumber"`
	StoreChainStoreID             string      `json:"StoreChainStoreId"`
	DeemedResellerCategory        string      `json:"DeemedResellerCategory"`
	BuyerInfo                     *OrderItemBuyerInfo `json:"BuyerInfo"`
	BuyerRequestedCancel          *BuyerRequestedCancel `json:"BuyerRequestedCancel"`
}

// ProductInfo represents product information
type ProductInfo struct {
	NumberOfItems int `json:"NumberOfItems"`
}

// PointsGranted represents points granted
type PointsGranted struct {
	PointsNumber        int         `json:"PointsNumber"`
	PointsMonetaryValue *OrderMoney `json:"PointsMonetaryValue"`
}

// TaxCollection represents tax collection info
type TaxCollection struct {
	Model                 string `json:"Model"`
	ResponsibleParty      string `json:"ResponsibleParty"`
}

// OrderItemBuyerInfo represents order item buyer info
type OrderItemBuyerInfo struct {
	BuyerCustomizedInfo *BuyerCustomizedInfo `json:"BuyerCustomizedInfo"`
	GiftWrapPrice       *OrderMoney          `json:"GiftWrapPrice"`
	GiftWrapTax         *OrderMoney          `json:"GiftWrapTax"`
	GiftMessageText     string               `json:"GiftMessageText"`
	GiftWrapLevel       string               `json:"GiftWrapLevel"`
}

// BuyerCustomizedInfo represents buyer customization
type BuyerCustomizedInfo struct {
	CustomizedURL string `json:"CustomizedURL"`
}

// BuyerRequestedCancel represents buyer cancellation request
type BuyerRequestedCancel struct {
	IsBuyerRequestedCancel bool   `json:"IsBuyerRequestedCancel"`
	BuyerCancelReason      string `json:"BuyerCancelReason"`
}

// GetOrders retrieves orders within a time range
func (c *Client) GetOrders(createdAfter, createdBefore time.Time, orderStatuses []string, nextToken string) ([]Order, string, error) {
	path := "/orders/v0/orders"

	params := map[string]string{
		"MarketplaceIds": c.MarketplaceID,
		"CreatedAfter":   createdAfter.Format(time.RFC3339),
	}

	if !createdBefore.IsZero() {
		params["CreatedBefore"] = createdBefore.Format(time.RFC3339)
	}

	if len(orderStatuses) > 0 {
		params["OrderStatuses"] = joinStrings(orderStatuses, ",")
	}

	if nextToken != "" {
		params["NextToken"] = nextToken
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, "", err
	}

	var result struct {
		Payload struct {
			Orders    []Order `json:"Orders"`
			NextToken string  `json:"NextToken"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse orders: %w", err)
	}

	return result.Payload.Orders, result.Payload.NextToken, nil
}

// GetOrder retrieves a single order by ID
func (c *Client) GetOrder(orderID string) (*Order, error) {
	path := fmt.Sprintf("/orders/v0/orders/%s", orderID)

	resp, err := c.Get(path, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Payload Order `json:"payload"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse order: %w", err)
	}

	return &result.Payload, nil
}

// GetOrderItems retrieves items for an order
func (c *Client) GetOrderItems(orderID string, nextToken string) ([]OrderItem, string, error) {
	path := fmt.Sprintf("/orders/v0/orders/%s/orderItems", orderID)

	params := map[string]string{}
	if nextToken != "" {
		params["NextToken"] = nextToken
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, "", err
	}

	var result struct {
		Payload struct {
			OrderItems []OrderItem `json:"OrderItems"`
			NextToken  string      `json:"NextToken"`
			AmazonOrderID string   `json:"AmazonOrderId"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse order items: %w", err)
	}

	return result.Payload.OrderItems, result.Payload.NextToken, nil
}

// GetOrderBuyerInfo retrieves buyer info for an order
func (c *Client) GetOrderBuyerInfo(orderID string) (*BuyerInfo, error) {
	path := fmt.Sprintf("/orders/v0/orders/%s/buyerInfo", orderID)

	resp, err := c.Get(path, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Payload struct {
			AmazonOrderID string     `json:"AmazonOrderId"`
			BuyerEmail    string     `json:"BuyerEmail"`
			BuyerName     string     `json:"BuyerName"`
			BuyerCounty   string     `json:"BuyerCounty"`
			BuyerTaxInfo  *TaxInformation `json:"BuyerTaxInfo"`
			PurchaseOrderNumber string `json:"PurchaseOrderNumber"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse buyer info: %w", err)
	}

	return &BuyerInfo{
		BuyerEmail:          result.Payload.BuyerEmail,
		BuyerName:           result.Payload.BuyerName,
		BuyerCounty:         result.Payload.BuyerCounty,
		BuyerTaxInfo:        result.Payload.BuyerTaxInfo,
		PurchaseOrderNumber: result.Payload.PurchaseOrderNumber,
	}, nil
}

// GetOrderAddress retrieves shipping address for an order
func (c *Client) GetOrderAddress(orderID string) (*Address, error) {
	path := fmt.Sprintf("/orders/v0/orders/%s/address", orderID)

	resp, err := c.Get(path, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Payload struct {
			AmazonOrderID   string   `json:"AmazonOrderId"`
			ShippingAddress *Address `json:"ShippingAddress"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse order address: %w", err)
	}

	return result.Payload.ShippingAddress, nil
}

// GetRecentOrders retrieves orders from the last N hours
func (c *Client) GetRecentOrders(hours int, orderStatuses []string) ([]Order, error) {
	createdAfter := time.Now().Add(-time.Duration(hours) * time.Hour)
	
	var allOrders []Order
	nextToken := ""

	for {
		orders, token, err := c.GetOrders(createdAfter, time.Time{}, orderStatuses, nextToken)
		if err != nil {
			return nil, err
		}

		allOrders = append(allOrders, orders...)

		if token == "" {
			break
		}
		nextToken = token
	}

	return allOrders, nil
}

// GetAllOrderItems retrieves all items for an order (handles pagination)
func (c *Client) GetAllOrderItems(orderID string) ([]OrderItem, error) {
	var allItems []OrderItem
	nextToken := ""

	for {
		items, token, err := c.GetOrderItems(orderID, nextToken)
		if err != nil {
			return nil, err
		}

		allItems = append(allItems, items...)

		if token == "" {
			break
		}
		nextToken = token
	}

	return allItems, nil
}
