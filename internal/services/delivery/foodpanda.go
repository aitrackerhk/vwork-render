package delivery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FoodpandaService Foodpanda 整合服務
type FoodpandaService struct {
	Config     IntegrationConfig
	BaseURL    string
	HTTPClient *http.Client
}

// Foodpanda API 端點
const (
	FoodpandaProductionURL = "https://partner-api.foodpanda.com"
	FoodpandaSandboxURL    = "https://partner-api.sandbox.foodpanda.com"
)

// FoodpandaOrderResponse Foodpanda 訂單響應
type FoodpandaOrderResponse struct {
	Success bool                   `json:"success"`
	Data    map[string]interface{} `json:"data"`
	Error   string                 `json:"error,omitempty"`
}

// NewFoodpandaService 創建 Foodpanda 服務
func NewFoodpandaService(config IntegrationConfig) *FoodpandaService {
	baseURL := FoodpandaProductionURL
	if config.Sandbox {
		baseURL = FoodpandaSandboxURL
	}

	// Allow overriding BaseURL from ExtraSettings (for testing)
	if url, ok := config.ExtraSettings["base_url"].(string); ok && url != "" {
		baseURL = url
	}

	return &FoodpandaService{
		Config:  config,
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest 執行 HTTP 請求
func (s *FoodpandaService) doRequest(method, path string, body interface{}) ([]byte, error) {
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

	// 設置認證頭
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Config.AccessToken)
	req.Header.Set("X-Vendor-ID", s.Config.MerchantID)

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
func (s *FoodpandaService) TestConnection() error {
	_, err := s.doRequest("GET", "/v1/vendor/info", nil)
	return err
}

// GetOrders 獲取訂單列表
func (s *FoodpandaService) GetOrders(since time.Time, limit int) ([]Order, error) {
	path := fmt.Sprintf("/v1/orders?since=%s&limit=%d", since.Format(time.RFC3339), limit)

	respBody, err := s.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析響應失敗: %w", err)
	}

	orders := make([]Order, 0, len(resp.Data))
	for _, orderData := range resp.Data {
		order := s.parseOrder(orderData)
		orders = append(orders, order)
	}

	return orders, nil
}

// GetOrder 獲取單個訂單
func (s *FoodpandaService) GetOrder(orderID string) (*Order, error) {
	path := fmt.Sprintf("/v1/orders/%s", orderID)

	respBody, err := s.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析響應失敗: %w", err)
	}

	order := s.parseOrder(resp.Data)
	return &order, nil
}

// AcceptOrder 接受訂單
func (s *FoodpandaService) AcceptOrder(orderID string) error {
	path := fmt.Sprintf("/v1/orders/%s/accept", orderID)
	_, err := s.doRequest("POST", path, nil)
	return err
}

// RejectOrder 拒絕訂單
func (s *FoodpandaService) RejectOrder(orderID string, reason string) error {
	path := fmt.Sprintf("/v1/orders/%s/reject", orderID)
	body := map[string]string{"reason": reason}
	_, err := s.doRequest("POST", path, body)
	return err
}

// MarkPreparing 標記為準備中
func (s *FoodpandaService) MarkPreparing(orderID string) error {
	path := fmt.Sprintf("/v1/orders/%s/preparing", orderID)
	_, err := s.doRequest("POST", path, nil)
	return err
}

// MarkReadyForPickup 標記為可取餐
func (s *FoodpandaService) MarkReadyForPickup(orderID string) error {
	path := fmt.Sprintf("/v1/orders/%s/ready", orderID)
	_, err := s.doRequest("POST", path, nil)
	return err
}

// SyncMenu 同步菜單
func (s *FoodpandaService) SyncMenu(products []MenuProduct) error {
	path := "/v1/menu/sync"
	_, err := s.doRequest("POST", path, map[string]interface{}{
		"products": products,
	})
	return err
}

// UpdateItemAvailability 更新商品可用狀態
func (s *FoodpandaService) UpdateItemAvailability(itemID string, available bool) error {
	path := fmt.Sprintf("/v1/menu/items/%s/availability", itemID)
	_, err := s.doRequest("PUT", path, map[string]bool{
		"available": available,
	})
	return err
}

// ValidateWebhook 驗證 Webhook 簽名
func (s *FoodpandaService) ValidateWebhook(signature string, payload []byte) bool {
	return ValidateHMACSignature(s.Config.WebhookSecret, signature, payload)
}

// ParseWebhookEvent 解析 Webhook 事件
func (s *FoodpandaService) ParseWebhookEvent(payload []byte) (*WebhookEvent, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("解析 webhook 失敗: %w", err)
	}

	event := &WebhookEvent{
		Platform:   PlatformFoodpanda,
		EventType:  getString(data, "event_type"),
		EventID:    getString(data, "event_id"),
		OrderID:    getString(data, "order_id"),
		RawPayload: payload,
		ParsedData: data,
		ReceivedAt: time.Now(),
	}

	// 映射狀態
	event.Status = s.mapStatus(getString(data, "status"))

	return event, nil
}

// RefreshToken 刷新 Token
func (s *FoodpandaService) RefreshToken() (string, string, *time.Time, error) {
	path := "/v1/auth/token/refresh"
	body := map[string]string{
		"refresh_token": s.Config.RefreshToken,
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
func (s *FoodpandaService) parseOrder(data map[string]interface{}) Order {
	order := Order{
		PlatformOrderID:     getString(data, "id"),
		PlatformOrderNumber: getString(data, "order_number"),
		Platform:            PlatformFoodpanda,
		PlatformStatus:      getString(data, "status"),
		CustomerName:        getString(data, "customer.name"),
		CustomerPhone:       getString(data, "customer.phone"),
		CustomerAddress:     getString(data, "customer.address"),
		CustomerNotes:       getString(data, "customer.notes"),
		DeliveryType:        getString(data, "delivery_type"),
		Subtotal:            getFloat(data, "subtotal"),
		DeliveryFee:         getFloat(data, "delivery_fee"),
		PlatformFee:         getFloat(data, "platform_fee"),
		DiscountAmount:      getFloat(data, "discount"),
		TotalAmount:         getFloat(data, "total"),
		Currency:            getString(data, "currency"),
		RawData:             data,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
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
					UnitPrice:      getFloat(itemMap, "unit_price"),
					TotalPrice:     getFloat(itemMap, "total_price"),
					Notes:          getString(itemMap, "notes"),
				}
				order.Items = append(order.Items, orderItem)
			}
		}
	}

	// 騎手信息
	if rider, ok := data["rider"].(map[string]interface{}); ok {
		order.RiderName = getString(rider, "name")
		order.RiderPhone = getString(rider, "phone")
		order.RiderTrackingURL = getString(rider, "tracking_url")
	}

	return order
}

// mapStatus 映射平台狀態到統一狀態
func (s *FoodpandaService) mapStatus(platformStatus string) OrderStatus {
	switch platformStatus {
	case "new", "pending":
		return OrderStatusPending
	case "accepted", "confirmed":
		return OrderStatusConfirmed
	case "preparing", "in_kitchen":
		return OrderStatusPreparing
	case "ready", "ready_for_pickup":
		return OrderStatusReadyForPickup
	case "picked_up", "on_the_way":
		return OrderStatusPickedUp
	case "delivered", "completed":
		return OrderStatusDelivered
	case "cancelled", "rejected":
		return OrderStatusCancelled
	case "failed":
		return OrderStatusFailed
	default:
		return OrderStatusPending
	}
}

// 輔助函數
func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getFloat(data map[string]interface{}, key string) float64 {
	if v, ok := data[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return 0
}

func getInt(data map[string]interface{}, key string) int {
	if v, ok := data[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}
