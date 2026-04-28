package hktvmall

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Order represents a HKTVmall order
type Order struct {
	OrderID       string       `json:"order_id"`
	OrderNo       string       `json:"order_no"`
	Status        string       `json:"status"` // pending, confirmed, shipped, delivered, cancelled
	TotalAmount   float64      `json:"total_amount"`
	Currency      string       `json:"currency"`
	Items         []OrderItem  `json:"items"`
	ShippingInfo  ShippingInfo `json:"shipping_info"`
	PaymentMethod string       `json:"payment_method"`
	CreateTime    int64        `json:"create_time"`
	UpdateTime    int64        `json:"update_time"`
	Remark        string       `json:"remark,omitempty"`
}

// OrderItem represents an order item
type OrderItem struct {
	ItemID      string  `json:"item_id"`
	ProductID   string  `json:"product_id"`
	ProductCode string  `json:"product_code"` // SKU
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	Price       float64 `json:"price"`
	TotalPrice  float64 `json:"total_price"`
}

// ShippingInfo represents shipping information
type ShippingInfo struct {
	RecipientName  string `json:"recipient_name"`
	Phone          string `json:"phone"`
	Address        string `json:"address"`
	District       string `json:"district"`
	City           string `json:"city"`
	PostalCode     string `json:"postal_code,omitempty"`
	ShippingMethod string `json:"shipping_method"`
	TrackingNo     string `json:"tracking_no,omitempty"`
}

// OrderListResponse represents the response from order list API
type OrderListResponse struct {
	Orders      []Order `json:"orders"`
	TotalCount  int     `json:"total_count"`
	CurrentPage int     `json:"current_page"`
	PageSize    int     `json:"page_size"`
	HasNextPage bool    `json:"has_next_page"`
}

// GetOrderList retrieves the list of orders
func (c *Client) GetOrderList(page, pageSize int, status string, startTime, endTime *time.Time) (*OrderListResponse, error) {
	params := map[string]string{
		"page":      strconv.Itoa(page),
		"page_size": strconv.Itoa(pageSize),
	}
	if status != "" {
		params["status"] = status
	}
	if startTime != nil {
		params["start_time"] = strconv.FormatInt(startTime.Unix(), 10)
	}
	if endTime != nil {
		params["end_time"] = strconv.FormatInt(endTime.Unix(), 10)
	}

	resp, err := c.Get("/api/v1/orders", params)
	if err != nil {
		return nil, err
	}

	var result OrderListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse order list: %w", err)
	}

	return &result, nil
}

// GetOrder retrieves a single order by ID
func (c *Client) GetOrder(orderID string) (*Order, error) {
	resp, err := c.Get("/api/v1/orders/"+orderID, nil)
	if err != nil {
		return nil, err
	}

	var order Order
	if err := json.Unmarshal(resp.Data, &order); err != nil {
		return nil, fmt.Errorf("failed to parse order: %w", err)
	}

	return &order, nil
}

// ConfirmOrderRequest represents the request to confirm an order
type ConfirmOrderRequest struct {
	OrderID string `json:"order_id"`
}

// ConfirmOrder confirms an order
func (c *Client) ConfirmOrder(orderID string) error {
	_, err := c.Post("/api/v1/orders/"+orderID+"/confirm", nil)
	return err
}

// ShipOrderRequest represents the request to ship an order
type ShipOrderRequest struct {
	TrackingNo     string `json:"tracking_no"`
	ShippingMethod string `json:"shipping_method,omitempty"`
}

// ShipOrder marks an order as shipped
func (c *Client) ShipOrder(orderID string, trackingNo string, shippingMethod string) error {
	req := ShipOrderRequest{
		TrackingNo:     trackingNo,
		ShippingMethod: shippingMethod,
	}
	_, err := c.Post("/api/v1/orders/"+orderID+"/ship", req)
	return err
}

// CancelOrderRequest represents the request to cancel an order
type CancelOrderRequest struct {
	Reason string `json:"reason"`
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(orderID string, reason string) error {
	req := CancelOrderRequest{Reason: reason}
	_, err := c.Post("/api/v1/orders/"+orderID+"/cancel", req)
	return err
}
