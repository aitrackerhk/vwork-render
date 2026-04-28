package delivery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeliverooService Deliveroo 整合服務
type DeliverooService struct {
	Config     IntegrationConfig
	BaseURL    string
	HTTPClient *http.Client
}

// Deliveroo API 端點
const (
	DeliverooProductionURL = "https://api.deliveroo.com"
	DeliverooSandboxURL    = "https://api.sandbox.deliveroo.com"
)

// NewDeliverooService 創建 Deliveroo 服務
func NewDeliverooService(config IntegrationConfig) *DeliverooService {
	baseURL := DeliverooProductionURL
	if config.Sandbox {
		baseURL = DeliverooSandboxURL
	}

	// Allow overriding BaseURL from ExtraSettings (for testing)
	if url, ok := config.ExtraSettings["base_url"].(string); ok && url != "" {
		baseURL = url
	}

	return &DeliverooService{
		Config:  config,
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest 執行 HTTP 請求
func (s *DeliverooService) doRequest(method, path string, body interface{}) ([]byte, error) {
	url := s.BaseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化請求失敗: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("創建請求失敗: %w", err)
	}

	// Deliveroo 使用 OAuth2 Bearer Token
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Config.AccessToken)
	req.Header.Set("X-Site-Id", s.Config.MerchantID)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("請求失敗: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("讀取響應失敗: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API 錯誤 (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// TestConnection 測試連接
func (s *DeliverooService) TestConnection() error {
	_, err := s.doRequest("GET", "/orderapp/v1/sites", nil)
	return err
}

// GetOrders 獲取訂單列表
func (s *DeliverooService) GetOrders(since time.Time, limit int) ([]Order, error) {
	path := fmt.Sprintf("/orderapp/v1/orders?since=%s&limit=%d", since.Format(time.RFC3339), limit)

	respBody, err := s.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Orders []map[string]interface{} `json:"orders"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析響應失敗: %w", err)
	}

	orders := make([]Order, 0, len(resp.Orders))
	for _, orderData := range resp.Orders {
		order := s.parseOrder(orderData)
		orders = append(orders, order)
	}

	return orders, nil
}

// GetOrder 獲取單個訂單
func (s *DeliverooService) GetOrder(orderID string) (*Order, error) {
	path := fmt.Sprintf("/orderapp/v1/orders/%s", orderID)

	respBody, err := s.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Order map[string]interface{} `json:"order"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析響應失敗: %w", err)
	}

	order := s.parseOrder(resp.Order)
	return &order, nil
}

// AcceptOrder 接受訂單
func (s *DeliverooService) AcceptOrder(orderID string) error {
	path := fmt.Sprintf("/orderapp/v1/orders/%s/accept", orderID)
	_, err := s.doRequest("POST", path, nil)
	return err
}

// RejectOrder 拒絕訂單
func (s *DeliverooService) RejectOrder(orderID string, reason string) error {
	path := fmt.Sprintf("/orderapp/v1/orders/%s/reject", orderID)
	body := map[string]string{"reason": reason}
	_, err := s.doRequest("POST", path, body)
	return err
}

// MarkPreparing 標記為準備中
func (s *DeliverooService) MarkPreparing(orderID string) error {
	path := fmt.Sprintf("/orderapp/v1/orders/%s/in-kitchen", orderID)
	_, err := s.doRequest("POST", path, nil)
	return err
}

// MarkReadyForPickup 標記為可取餐
func (s *DeliverooService) MarkReadyForPickup(orderID string) error {
	path := fmt.Sprintf("/orderapp/v1/orders/%s/ready-for-collection", orderID)
	_, err := s.doRequest("POST", path, nil)
	return err
}

// SyncMenu 同步菜單
func (s *DeliverooService) SyncMenu(products []MenuProduct) error {
	path := "/menu/v2/items"

	// Deliveroo 菜單 API 格式
	items := make([]map[string]interface{}, 0, len(products))
	for _, p := range products {
		items = append(items, map[string]interface{}{
			"id":          p.ID,
			"name":        p.Name,
			"description": p.Description,
			"price":       int(p.Price * 100), // Deliveroo 使用分為單位
			"image_url":   p.ImageURL,
			"available":   p.Available,
		})
	}

	_, err := s.doRequest("PUT", path, map[string]interface{}{
		"items": items,
	})
	return err
}

// UpdateItemAvailability 更新商品可用狀態
func (s *DeliverooService) UpdateItemAvailability(itemID string, available bool) error {
	path := fmt.Sprintf("/menu/v2/items/%s/availability", itemID)
	_, err := s.doRequest("PUT", path, map[string]bool{
		"available": available,
	})
	return err
}

// ValidateWebhook 驗證 Webhook 簽名
func (s *DeliverooService) ValidateWebhook(signature string, payload []byte) bool {
	return ValidateHMACSignature(s.Config.WebhookSecret, signature, payload)
}

// ParseWebhookEvent 解析 Webhook 事件
func (s *DeliverooService) ParseWebhookEvent(payload []byte) (*WebhookEvent, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("解析 webhook 失敗: %w", err)
	}

	event := &WebhookEvent{
		Platform:   PlatformDeliveroo,
		EventType:  getString(data, "type"),
		EventID:    getString(data, "id"),
		RawPayload: payload,
		ParsedData: data,
		ReceivedAt: time.Now(),
	}

	// 從 data.order 獲取訂單信息
	if orderData, ok := data["order"].(map[string]interface{}); ok {
		event.OrderID = getString(orderData, "id")
		event.Status = s.mapStatus(getString(orderData, "status"))
	}

	return event, nil
}

// RefreshToken 刷新 Token
func (s *DeliverooService) RefreshToken() (string, string, *time.Time, error) {
	// Deliveroo OAuth2 Token 刷新
	path := "/oauth2/token"
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": s.Config.RefreshToken,
		"client_id":     s.Config.APIKey,
		"client_secret": s.Config.APISecret,
	}

	respBody, err := s.doRequest("POST", path, body)
	if err != nil {
		return "", "", nil, err
	}

	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", "", nil, fmt.Errorf("解析響應失敗: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	return resp.AccessToken, resp.RefreshToken, &expiresAt, nil
}

// parseOrder 解析訂單數據
func (s *DeliverooService) parseOrder(data map[string]interface{}) Order {
	order := Order{
		PlatformOrderID:     getString(data, "id"),
		PlatformOrderNumber: getString(data, "display_id"),
		Platform:            PlatformDeliveroo,
		PlatformStatus:      getString(data, "status"),
		DeliveryType:        getString(data, "fulfillment_type"),
		Currency:            getString(data, "currency"),
		RawData:             data,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	// 客戶信息
	if customer, ok := data["customer"].(map[string]interface{}); ok {
		order.CustomerName = getString(customer, "name")
		order.CustomerPhone = getString(customer, "phone_number")
	}

	// 配送地址
	if address, ok := data["delivery_address"].(map[string]interface{}); ok {
		order.CustomerAddress = fmt.Sprintf("%s, %s, %s",
			getString(address, "address_line1"),
			getString(address, "address_line2"),
			getString(address, "postcode"))
	}

	// 金額信息
	if pricing, ok := data["pricing"].(map[string]interface{}); ok {
		// Deliveroo 金額以分為單位
		order.Subtotal = getFloat(pricing, "food_total") / 100
		order.DeliveryFee = getFloat(pricing, "delivery_fee") / 100
		order.PlatformFee = getFloat(pricing, "service_fee") / 100
		order.DiscountAmount = getFloat(pricing, "discount") / 100
		order.TotalAmount = getFloat(pricing, "total") / 100
	}

	// 映射狀態
	order.Status = s.mapStatus(order.PlatformStatus)

	// 解析訂單項目
	if items, ok := data["items"].([]interface{}); ok {
		for _, item := range items {
			if itemMap, ok := item.(map[string]interface{}); ok {
				orderItem := OrderItem{
					PlatformItemID: getString(itemMap, "id"),
					Name:           getString(itemMap, "name"),
					Quantity:       getInt(itemMap, "quantity"),
					UnitPrice:      getFloat(itemMap, "unit_price") / 100,
					TotalPrice:     getFloat(itemMap, "total_price") / 100,
					Notes:          getString(itemMap, "special_instructions"),
				}

				// 解析修改項（配料）
				if modifiers, ok := itemMap["modifiers"].([]interface{}); ok {
					for _, mod := range modifiers {
						if modMap, ok := mod.(map[string]interface{}); ok {
							orderItem.Modifiers = append(orderItem.Modifiers, OrderModifier{
								Name:     getString(modMap, "name"),
								Price:    getFloat(modMap, "price") / 100,
								Quantity: getInt(modMap, "quantity"),
							})
						}
					}
				}

				order.Items = append(order.Items, orderItem)
			}
		}
	}

	// 騎手信息
	if rider, ok := data["rider"].(map[string]interface{}); ok {
		order.RiderName = getString(rider, "name")
		order.RiderPhone = getString(rider, "phone_number")
		order.RiderTrackingURL = getString(rider, "tracking_url")
	}

	// 備註
	order.CustomerNotes = getString(data, "notes")

	return order
}

// mapStatus 映射平台狀態到統一狀態
func (s *DeliverooService) mapStatus(platformStatus string) OrderStatus {
	switch platformStatus {
	case "pending", "awaiting_confirmation":
		return OrderStatusPending
	case "accepted", "confirmed":
		return OrderStatusConfirmed
	case "in_kitchen", "preparing":
		return OrderStatusPreparing
	case "ready_for_collection", "ready":
		return OrderStatusReadyForPickup
	case "collected", "on_route":
		return OrderStatusPickedUp
	case "delivered", "completed":
		return OrderStatusDelivered
	case "rejected", "cancelled":
		return OrderStatusCancelled
	case "failed":
		return OrderStatusFailed
	default:
		return OrderStatusPending
	}
}
