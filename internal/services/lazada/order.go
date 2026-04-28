package lazada

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Order represents a Lazada order
type Order struct {
	OrderID           int64         `json:"order_id"`
	OrderNumber       string        `json:"order_number"`
	CustomerFirstName string        `json:"customer_first_name"`
	CustomerLastName  string        `json:"customer_last_name"`
	PaymentMethod     string        `json:"payment_method"`
	Remarks           string        `json:"remarks"`
	DeliveryInfo      string        `json:"delivery_info"`
	Price             string        `json:"price"`
	GiftOption        bool          `json:"gift_option"`
	GiftMessage       string        `json:"gift_message"`
	VoucherPlatform   float64       `json:"voucher_platform"`
	VoucherSeller     float64       `json:"voucher_seller"`
	VoucherCode       string        `json:"voucher_code"`
	CreatedAt         string        `json:"created_at"`
	UpdatedAt         string        `json:"updated_at"`
	AddressBilling    OrderAddress  `json:"address_billing"`
	AddressShipping   OrderAddress  `json:"address_shipping"`
	ItemsCount        int           `json:"items_count"`
	Statuses          []string      `json:"statuses"`
	BranchNumber      string        `json:"branch_number"`
	TaxCode           string        `json:"tax_code"`
	PromisedShippingTime string     `json:"promised_shipping_time"`
	ExtraAttributes   string        `json:"extra_attributes"`
	NationalRegistrationNumber string `json:"national_registration_number"`
}

// OrderAddress represents an order address
type OrderAddress struct {
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Phone       string `json:"phone"`
	Phone2      string `json:"phone2"`
	Address1    string `json:"address1"`
	Address2    string `json:"address2"`
	Address3    string `json:"address3"`
	Address4    string `json:"address4"`
	Address5    string `json:"address5"`
	City        string `json:"city"`
	PostCode    string `json:"post_code"`
	Country     string `json:"country"`
}

// OrderItem represents an order item
type OrderItem struct {
	OrderItemID         int64   `json:"order_item_id"`
	OrderID             int64   `json:"order_id"`
	ShopID              string  `json:"shop_id"`
	Name                string  `json:"name"`
	Sku                 string  `json:"sku"`
	SkuID               int64   `json:"sku_id"`
	Variation           string  `json:"variation"`
	ShopSku             string  `json:"shop_sku"`
	ShippingType        string  `json:"shipping_type"`
	ItemPrice           float64 `json:"item_price"`
	PaidPrice           float64 `json:"paid_price"`
	Currency            string  `json:"currency"`
	WalletCredits       float64 `json:"wallet_credits"`
	TaxAmount           float64 `json:"tax_amount"`
	ShippingFeeOriginal float64 `json:"shipping_fee_original"`
	ShippingFeeDiscountSeller  float64 `json:"shipping_fee_discount_seller"`
	ShippingFeeDiscountPlatform float64 `json:"shipping_fee_discount_platform"`
	ShippingServiceCost float64 `json:"shipping_service_cost"`
	VoucherSeller       float64 `json:"voucher_seller"`
	VoucherPlatform     float64 `json:"voucher_platform"`
	VoucherSellerLpi    float64 `json:"voucher_seller_lpi"`
	VoucherPlatformLpi  float64 `json:"voucher_platform_lpi"`
	Status              string  `json:"status"`
	IsProcessable       bool    `json:"is_processable"`
	ShipmentProvider    string  `json:"shipment_provider"`
	TrackingCode        string  `json:"tracking_code"`
	TrackingCodePre     string  `json:"tracking_code_pre"`
	Reason              string  `json:"reason"`
	ReasonDetail        string  `json:"reason_detail"`
	PurchaseOrderID     string  `json:"purchase_order_id"`
	PurchaseOrderNumber string  `json:"purchase_order_number"`
	PackageID           string  `json:"package_id"`
	PromisedShippingTime string `json:"promised_shipping_time"`
	ExtraAttributes     string  `json:"extra_attributes"`
	ShippingAmount      float64 `json:"shipping_amount"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
	ReturnStatus        string  `json:"return_status"`
	ProductMainImage    string  `json:"product_main_image"`
	ProductDetailUrl    string  `json:"product_detail_url"`
	InvoiceNumber       string  `json:"invoice_number"`
	DigitalDeliveryInfo string  `json:"digital_delivery_info"`
	ExtraAttributes2    string  `json:"extra_attributes_2"`
	BuyerID             int64   `json:"buyer_id"`
}

// OrdersResponse represents get orders response
type OrdersResponse struct {
	Count      int     `json:"count"`
	CountTotal int     `json:"countTotal"`
	Orders     []Order `json:"orders"`
}

// GetOrders gets orders with filters
func (c *Client) GetOrders(createdAfter, createdBefore time.Time, status string, offset, limit int) (*OrdersResponse, error) {
	params := map[string]string{
		"offset": strconv.Itoa(offset),
		"limit":  strconv.Itoa(limit),
	}

	if !createdAfter.IsZero() {
		params["created_after"] = createdAfter.Format(time.RFC3339)
	}
	if !createdBefore.IsZero() {
		params["created_before"] = createdBefore.Format(time.RFC3339)
	}
	if status != "" {
		params["status"] = status
	}

	resp, err := c.Get("/orders/get", params)
	if err != nil {
		return nil, err
	}

	var result OrdersResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse orders: %w", err)
	}

	return &result, nil
}

// GetOrder gets a single order by ID
func (c *Client) GetOrder(orderID int64) (*Order, error) {
	params := map[string]string{
		"order_id": strconv.FormatInt(orderID, 10),
	}

	resp, err := c.Get("/order/get", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data Order `json:"data"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		// Try direct parse
		var order Order
		if err := json.Unmarshal(resp.Data, &order); err != nil {
			return nil, fmt.Errorf("failed to parse order: %w", err)
		}
		return &order, nil
	}

	return &result.Data, nil
}

// GetOrderItems gets items for an order
func (c *Client) GetOrderItems(orderID int64) ([]OrderItem, error) {
	params := map[string]string{
		"order_id": strconv.FormatInt(orderID, 10),
	}

	resp, err := c.Get("/order/items/get", params)
	if err != nil {
		return nil, err
	}

	var items []OrderItem
	if err := json.Unmarshal(resp.Data, &items); err != nil {
		return nil, fmt.Errorf("failed to parse order items: %w", err)
	}

	return items, nil
}

// GetMultipleOrderItems gets items for multiple orders
func (c *Client) GetMultipleOrderItems(orderIDs []int64) ([]OrderItem, error) {
	// Convert order IDs to JSON array
	ids := make([]string, len(orderIDs))
	for i, id := range orderIDs {
		ids[i] = strconv.FormatInt(id, 10)
	}
	orderList := "[" + strJoin(ids, ",") + "]"

	params := map[string]string{
		"order_ids": orderList,
	}

	resp, err := c.Get("/orders/items/get", params)
	if err != nil {
		return nil, err
	}

	var items []OrderItem
	if err := json.Unmarshal(resp.Data, &items); err != nil {
		return nil, fmt.Errorf("failed to parse order items: %w", err)
	}

	return items, nil
}

// GetRecentOrders gets orders created in the last N hours
func (c *Client) GetRecentOrders(hours int, status string) ([]Order, error) {
	createdAfter := time.Now().Add(-time.Duration(hours) * time.Hour)
	
	var allOrders []Order
	offset := 0
	limit := 100

	for {
		resp, err := c.GetOrders(createdAfter, time.Time{}, status, offset, limit)
		if err != nil {
			return nil, err
		}

		allOrders = append(allOrders, resp.Orders...)

		if len(resp.Orders) < limit {
			break
		}
		offset += limit
	}

	return allOrders, nil
}

// SetStatusToPackedByMarketplace sets order status to packed
func (c *Client) SetStatusToPackedByMarketplace(orderItemIDs []int64, shipmentProvider, trackingNumber string) error {
	// Convert order item IDs to JSON array
	ids := make([]string, len(orderItemIDs))
	for i, id := range orderItemIDs {
		ids[i] = strconv.FormatInt(id, 10)
	}
	orderItemList := "[" + strJoin(ids, ",") + "]"

	params := map[string]string{
		"order_item_ids":    orderItemList,
		"delivery_type":     "dropship",
		"shipment_provider": shipmentProvider,
		"tracking_number":   trackingNumber,
	}

	_, err := c.Post("/order/pack", params, nil)
	return err
}

// SetStatusToReadyToShip sets order status to ready to ship
func (c *Client) SetStatusToReadyToShip(orderItemIDs []int64, shipmentProvider, trackingNumber string) error {
	ids := make([]string, len(orderItemIDs))
	for i, id := range orderItemIDs {
		ids[i] = strconv.FormatInt(id, 10)
	}
	orderItemList := "[" + strJoin(ids, ",") + "]"

	params := map[string]string{
		"order_item_ids":    orderItemList,
		"delivery_type":     "dropship",
		"shipment_provider": shipmentProvider,
		"tracking_number":   trackingNumber,
	}

	_, err := c.Post("/order/rts", params, nil)
	return err
}

// CancelOrder cancels an order item
func (c *Client) CancelOrder(orderItemID int64, reasonDetail, reasonID string) error {
	params := map[string]string{
		"order_item_id": strconv.FormatInt(orderItemID, 10),
		"reason_detail": reasonDetail,
		"reason_id":     reasonID,
	}

	_, err := c.Post("/order/cancel", params, nil)
	return err
}

// Helper function
func strJoin(items []string, sep string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for i := 1; i < len(items); i++ {
		result += sep + items[i]
	}
	return result
}

// Order status constants
const (
	OrderStatusPending          = "pending"
	OrderStatusCanceled         = "canceled"
	OrderStatusReadyToShip      = "ready_to_ship"
	OrderStatusDelivered        = "delivered"
	OrderStatusReturned         = "returned"
	OrderStatusShipped          = "shipped"
	OrderStatusFailed           = "failed"
	OrderStatusToProcess        = "toProcess"
	OrderStatusToReceive        = "toReceive"
	OrderStatusToReturn         = "toReturn"
)
