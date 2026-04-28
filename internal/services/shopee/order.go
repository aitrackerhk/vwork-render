package shopee

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Order represents a Shopee order
type Order struct {
	OrderSN               string            `json:"order_sn"`
	Region                string            `json:"region"`
	Currency              string            `json:"currency"`
	COD                   bool              `json:"cod"`
	TotalAmount           float64           `json:"total_amount"`
	OrderStatus           string            `json:"order_status"`
	ShippingCarrier       string            `json:"shipping_carrier"`
	PaymentMethod         string            `json:"payment_method"`
	EstimatedShippingFee  float64           `json:"estimated_shipping_fee"`
	MessageToSeller       string            `json:"message_to_seller"`
	CreateTime            int64             `json:"create_time"`
	UpdateTime            int64             `json:"update_time"`
	DaysToShip            int               `json:"days_to_ship"`
	ShipByDate            int64             `json:"ship_by_date"`
	BuyerUserID           int64             `json:"buyer_user_id"`
	BuyerUsername         string            `json:"buyer_username"`
	RecipientAddress      RecipientAddress  `json:"recipient_address"`
	ActualShippingFee     float64           `json:"actual_shipping_fee"`
	GoodsToDeclare        bool              `json:"goods_to_declare"`
	Note                  string            `json:"note"`
	NoteUpdateTime        int64             `json:"note_update_time"`
	ItemList              []OrderItem       `json:"item_list"`
	PayTime               int64             `json:"pay_time"`
	Dropshipper           string            `json:"dropshipper"`
	CreditCardNumber      string            `json:"credit_card_number"`
	DropshipperPhone      string            `json:"dropshipper_phone"`
	SplitUp               bool              `json:"split_up"`
	BuyerCancelReason     string            `json:"buyer_cancel_reason"`
	CancelBy              string            `json:"cancel_by"`
	CancelReason          string            `json:"cancel_reason"`
	ActualShippingFeeConfirmed bool         `json:"actual_shipping_fee_confirmed"`
	BuyerCPFID            string            `json:"buyer_cpf_id"`
	FulfillmentFlag       string            `json:"fulfillment_flag"`
	PickupDoneTime        int64             `json:"pickup_done_time"`
	PackageList           []PackageInfo     `json:"package_list"`
	InvoiceData           InvoiceData       `json:"invoice_data"`
	CheckoutShippingCarrier string          `json:"checkout_shipping_carrier"`
	ReverseShippingFee    float64           `json:"reverse_shipping_fee"`
	OrderChargeableWeight float64           `json:"order_chargeable_weight_gram"`
}

// RecipientAddress represents the shipping address
type RecipientAddress struct {
	Name        string `json:"name"`
	Phone       string `json:"phone"`
	Town        string `json:"town"`
	District    string `json:"district"`
	City        string `json:"city"`
	State       string `json:"state"`
	Region      string `json:"region"`
	Zipcode     string `json:"zipcode"`
	FullAddress string `json:"full_address"`
}

// OrderItem represents an item in an order
type OrderItem struct {
	ItemID               int64   `json:"item_id"`
	ItemName             string  `json:"item_name"`
	ItemSKU              string  `json:"item_sku"`
	ModelID              int64   `json:"model_id"`
	ModelName            string  `json:"model_name"`
	ModelSKU             string  `json:"model_sku"`
	ModelQuantityPurchased int   `json:"model_quantity_purchased"`
	ModelOriginalPrice   float64 `json:"model_original_price"`
	ModelDiscountedPrice float64 `json:"model_discounted_price"`
	Wholesale            bool    `json:"wholesale"`
	Weight               float64 `json:"weight"`
	AddOnDeal            bool    `json:"add_on_deal"`
	MainItem             bool    `json:"main_item"`
	AddOnDealID          int64   `json:"add_on_deal_id"`
	PromotionType        string  `json:"promotion_type"`
	PromotionID          int64   `json:"promotion_id"`
	OrderItemID          int64   `json:"order_item_id"`
	PromotionGroupID     int64   `json:"promotion_group_id"`
	ImageInfo            ImageInfo `json:"image_info"`
	ProductLocationID    []string `json:"product_location_id"`
}

// ImageInfo represents image information
type ImageInfo struct {
	ImageURL string `json:"image_url"`
}

// PackageInfo represents package information
type PackageInfo struct {
	PackageNumber       string           `json:"package_number"`
	LogisticsStatus     string           `json:"logistics_status"`
	ShippingCarrier     string           `json:"shipping_carrier"`
	ItemList            []PackageItem    `json:"item_list"`
	ParcelChargeableWeight float64       `json:"parcel_chargeable_weight_gram"`
}

// PackageItem represents an item in a package
type PackageItem struct {
	ItemID       int64 `json:"item_id"`
	ModelID      int64 `json:"model_id"`
	ModelQuantity int  `json:"model_quantity"`
}

// InvoiceData represents invoice information
type InvoiceData struct {
	Number            string `json:"number"`
	SeriesNumber      string `json:"series_number"`
	AccessKey         string `json:"access_key"`
	IssueDate         int64  `json:"issue_date"`
	TotalValue        float64 `json:"total_value"`
	ProductsTotalValue float64 `json:"products_total_value"`
	TaxCode           string `json:"tax_code"`
}

// GetOrderListResponse represents the response from get_order_list API
type GetOrderListResponse struct {
	More       bool         `json:"more"`
	NextCursor string       `json:"next_cursor"`
	OrderList  []OrderBasic `json:"order_list"`
}

// OrderBasic represents basic order info
type OrderBasic struct {
	OrderSN string `json:"order_sn"`
}

// GetOrderList retrieves the list of orders
func (c *Client) GetOrderList(timeRangeField string, timeFrom, timeTo int64, pageSize int, cursor string, orderStatus string) (*GetOrderListResponse, error) {
	path := "/api/v2/order/get_order_list"

	params := map[string]string{
		"time_range_field": timeRangeField,
		"time_from":        strconv.FormatInt(timeFrom, 10),
		"time_to":          strconv.FormatInt(timeTo, 10),
		"page_size":        strconv.Itoa(pageSize),
	}

	if cursor != "" {
		params["cursor"] = cursor
	}

	if orderStatus != "" {
		params["order_status"] = orderStatus
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, err
	}

	var result GetOrderListResponse
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("failed to parse order list: %w", err)
	}

	return &result, nil
}

// GetOrderDetail retrieves detailed order information
func (c *Client) GetOrderDetail(orderSNList []string, responseOptionalFields []string) ([]Order, error) {
	if len(orderSNList) == 0 {
		return nil, nil
	}
	if len(orderSNList) > 50 {
		return nil, fmt.Errorf("maximum 50 orders per request")
	}

	path := "/api/v2/order/get_order_detail"

	// Convert order SNs to comma-separated string
	orderSNsStr := ""
	for i, sn := range orderSNList {
		if i > 0 {
			orderSNsStr += ","
		}
		orderSNsStr += sn
	}

	params := map[string]string{
		"order_sn_list": orderSNsStr,
	}

	if len(responseOptionalFields) > 0 {
		fieldsStr := ""
		for i, field := range responseOptionalFields {
			if i > 0 {
				fieldsStr += ","
			}
			fieldsStr += field
		}
		params["response_optional_fields"] = fieldsStr
	}

	resp, err := c.Get(path, params)
	if err != nil {
		return nil, err
	}

	var result struct {
		OrderList []Order `json:"order_list"`
	}
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("failed to parse order detail: %w", err)
	}

	return result.OrderList, nil
}

// GetRecentOrders retrieves orders from the last N days
func (c *Client) GetRecentOrders(days int, orderStatus string) ([]Order, error) {
	now := time.Now()
	timeTo := now.Unix()
	timeFrom := now.AddDate(0, 0, -days).Unix()

	var allOrderSNs []string
	cursor := ""
	pageSize := 50

	// Get all order SNs
	for {
		listResp, err := c.GetOrderList("create_time", timeFrom, timeTo, pageSize, cursor, orderStatus)
		if err != nil {
			return nil, err
		}

		for _, order := range listResp.OrderList {
			allOrderSNs = append(allOrderSNs, order.OrderSN)
		}

		if !listResp.More {
			break
		}

		cursor = listResp.NextCursor
	}

	if len(allOrderSNs) == 0 {
		return nil, nil
	}

	// Get order details in batches of 50
	var allOrders []Order
	optionalFields := []string{
		"buyer_user_id", "buyer_username", "estimated_shipping_fee",
		"recipient_address", "actual_shipping_fee", "item_list",
		"pay_time", "dropshipper", "dropshipper_phone", "split_up",
		"buyer_cancel_reason", "cancel_by", "cancel_reason",
		"fulfillment_flag", "pickup_done_time", "package_list",
		"invoice_data", "note", "note_update_time",
	}

	for i := 0; i < len(allOrderSNs); i += 50 {
		end := i + 50
		if end > len(allOrderSNs) {
			end = len(allOrderSNs)
		}

		orders, err := c.GetOrderDetail(allOrderSNs[i:end], optionalFields)
		if err != nil {
			return nil, err
		}

		allOrders = append(allOrders, orders...)
	}

	return allOrders, nil
}

// Order statuses
const (
	OrderStatusUnpaid       = "UNPAID"
	OrderStatusReadyToShip  = "READY_TO_SHIP"
	OrderStatusProcessed    = "PROCESSED"
	OrderStatusShipped      = "SHIPPED"
	OrderStatusCompleted    = "COMPLETED"
	OrderStatusInCancel     = "IN_CANCEL"
	OrderStatusCancelled    = "CANCELLED"
	OrderStatusInvoicePending = "INVOICE_PENDING"
)
