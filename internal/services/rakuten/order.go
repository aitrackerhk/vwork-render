package rakuten

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Order represents a Rakuten order
type Order struct {
	OrderNumber        string         `json:"orderNumber"`        // 注文番号
	OrderDate          string         `json:"orderDate"`          // 注文日時
	OrderStatus        string         `json:"orderStatus"`        // 注文ステータス
	PaymentStatus      string         `json:"paymentStatus"`      // 入金ステータス
	ShippingStatus     string         `json:"shippingStatus"`     // 発送ステータス
	PaymentMethod      string         `json:"paymentMethod"`      // 支払方法
	TotalPrice         int            `json:"totalPrice"`         // 合計金額
	TotalItemPrice     int            `json:"totalItemPrice"`     // 商品合計
	ShippingFee        int            `json:"shippingFee"`        // 送料
	Tax                int            `json:"tax"`                // 消費税
	Points             int            `json:"points"`             // 使用ポイント
	Coupon             int            `json:"coupon"`             // クーポン利用額
	OrdererInfo        OrdererInfo    `json:"ordererInfo"`        // 注文者情報
	DeliveryInfo       DeliveryInfo   `json:"deliveryInfo"`       // 配送先情報
	Items              []OrderItem    `json:"items"`              // 注文商品
	ShippingInfo       ShippingInfo   `json:"shippingInfo"`       // 配送情報
	WrappingInfo       WrappingInfo   `json:"wrappingInfo"`       // ラッピング情報
	Remarks            string         `json:"remarks"`            // 備考
	ShopRemarks        string         `json:"shopRemarks"`        // ショップ備考
	CreatedTime        string         `json:"createdTime"`
	UpdatedTime        string         `json:"updatedTime"`
}

// OrdererInfo represents orderer information
type OrdererInfo struct {
	Name          string `json:"name"`
	NameKana      string `json:"nameKana"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	PostalCode    string `json:"postalCode"`
	Prefecture    string `json:"prefecture"`
	City          string `json:"city"`
	Address       string `json:"address"`
	Building      string `json:"building"`
}

// DeliveryInfo represents delivery address information
type DeliveryInfo struct {
	Name          string `json:"name"`
	NameKana      string `json:"nameKana"`
	Phone         string `json:"phone"`
	PostalCode    string `json:"postalCode"`
	Prefecture    string `json:"prefecture"`
	City          string `json:"city"`
	Address       string `json:"address"`
	Building      string `json:"building"`
}

// OrderItem represents an order item
type OrderItem struct {
	ItemNumber      string  `json:"itemNumber"`
	ManageNumber    string  `json:"manageNumber"`
	ItemName        string  `json:"itemName"`
	VariantID       string  `json:"variantId"`
	VariantName     string  `json:"variantName"`
	Quantity        int     `json:"quantity"`
	UnitPrice       int     `json:"unitPrice"`
	TotalPrice      int     `json:"totalPrice"`
	Points          int     `json:"points"`
	Tax             int     `json:"tax"`
	SKU             string  `json:"sku"`
	JAN             string  `json:"jan"`
	ImageURL        string  `json:"imageUrl"`
}

// ShippingInfo represents shipping information
type ShippingInfo struct {
	DeliveryCompany   string `json:"deliveryCompany"`
	TrackingNumber    string `json:"trackingNumber"`
	ShippingDate      string `json:"shippingDate"`
	DeliveryDate      string `json:"deliveryDate"`
	DeliveryTime      string `json:"deliveryTime"`
}

// WrappingInfo represents gift wrapping information
type WrappingInfo struct {
	WrappingFlag    bool   `json:"wrappingFlag"`
	NoshiFlag       bool   `json:"noshiFlag"`
	NoshiName       string `json:"noshiName"`
	MessageCardFlag bool   `json:"messageCardFlag"`
	Message         string `json:"message"`
}

// OrdersResponse represents get orders response
type OrdersResponse struct {
	TotalCount int     `json:"totalCount"`
	Offset     int     `json:"offset"`
	Limit      int     `json:"limit"`
	Orders     []Order `json:"orders"`
}

// GetOrders gets orders with filters
func (c *Client) GetOrders(startDate, endDate time.Time, status string, offset, limit int) (*OrdersResponse, error) {
	params := map[string]string{
		"offset": strconv.Itoa(offset),
		"limit":  strconv.Itoa(limit),
	}

	if !startDate.IsZero() {
		params["startDate"] = startDate.Format("2006-01-02T15:04:05+09:00")
	}
	if !endDate.IsZero() {
		params["endDate"] = endDate.Format("2006-01-02T15:04:05+09:00")
	}
	if status != "" {
		params["orderStatus"] = status
	}

	resp, err := c.Get("/orders/search", params)
	if err != nil {
		return nil, err
	}

	var result OrdersResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse orders: %w", err)
	}

	return &result, nil
}

// GetOrder gets a single order by order number
func (c *Client) GetOrder(orderNumber string) (*Order, error) {
	resp, err := c.Get("/orders/"+orderNumber, nil)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(resp.Data, &order); err != nil {
		return nil, fmt.Errorf("failed to parse order: %w", err)
	}

	return &order, nil
}

// GetRecentOrders gets orders from the last N hours
func (c *Client) GetRecentOrders(hours int, status string) ([]Order, error) {
	startDate := time.Now().Add(-time.Duration(hours) * time.Hour)

	var allOrders []Order
	offset := 0
	limit := 100

	for {
		resp, err := c.GetOrders(startDate, time.Time{}, status, offset, limit)
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

// UpdateOrderStatus updates order status
func (c *Client) UpdateOrderStatus(orderNumber, status string) error {
	body := map[string]interface{}{
		"orderStatus": status,
	}

	_, err := c.Patch("/orders/"+orderNumber+"/status", body)
	return err
}

// UpdateShippingInfo updates shipping information
func (c *Client) UpdateShippingInfo(orderNumber string, info ShippingInfo) error {
	_, err := c.Patch("/orders/"+orderNumber+"/shipping", info)
	return err
}

// ConfirmOrder confirms an order (processing start)
func (c *Client) ConfirmOrder(orderNumber string) error {
	return c.UpdateOrderStatus(orderNumber, "processing")
}

// ShipOrder marks order as shipped
func (c *Client) ShipOrder(orderNumber, deliveryCompany, trackingNumber string) error {
	info := ShippingInfo{
		DeliveryCompany: deliveryCompany,
		TrackingNumber:  trackingNumber,
		ShippingDate:    time.Now().Format("2006-01-02"),
	}

	if err := c.UpdateShippingInfo(orderNumber, info); err != nil {
		return fmt.Errorf("failed to update shipping info: %w", err)
	}

	return c.UpdateOrderStatus(orderNumber, "shipped")
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(orderNumber, reason string) error {
	body := map[string]interface{}{
		"orderStatus":  "cancelled",
		"cancelReason": reason,
	}

	_, err := c.Patch("/orders/"+orderNumber+"/cancel", body)
	return err
}

// Order status constants
const (
	OrderStatusNew        = "new"        // 新規
	OrderStatusProcessing = "processing" // 処理中
	OrderStatusShipped    = "shipped"    // 発送済
	OrderStatusDelivered  = "delivered"  // 配送完了
	OrderStatusCancelled  = "cancelled"  // キャンセル
	OrderStatusReturned   = "returned"   // 返品
	OrderStatusHold       = "hold"       // 保留
)

// Payment status constants
const (
	PaymentStatusPending   = "pending"   // 未入金
	PaymentStatusPaid      = "paid"      // 入金済
	PaymentStatusRefunded  = "refunded"  // 返金済
)

// Shipping status constants
const (
	ShippingStatusPending   = "pending"   // 未発送
	ShippingStatusShipped   = "shipped"   // 発送済
	ShippingStatusDelivered = "delivered" // 配送完了
)

// AddOrderMemo adds a memo/note to an order
func (c *Client) AddOrderMemo(orderNumber, memo string) error {
	body := map[string]interface{}{
		"shopRemarks": memo,
	}

	_, err := c.Patch("/orders/"+orderNumber+"/memo", body)
	return err
}

// GetOrderSummary gets order summary/statistics
func (c *Client) GetOrderSummary(startDate, endDate time.Time) (map[string]interface{}, error) {
	params := map[string]string{}
	if !startDate.IsZero() {
		params["startDate"] = startDate.Format("2006-01-02")
	}
	if !endDate.IsZero() {
		params["endDate"] = endDate.Format("2006-01-02")
	}

	resp, err := c.Get("/orders/summary", params)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse summary: %w", err)
	}

	return data, nil
}
