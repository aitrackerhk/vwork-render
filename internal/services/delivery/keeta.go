package delivery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// KeetaService Keeta (美團外賣香港版) 整合服務
type KeetaService struct {
	Config     IntegrationConfig
	BaseURL    string
	HTTPClient *http.Client
}

// Keeta API 端點
const (
	KeetaProductionURL = "https://openapi.keeta.com"
	KeetaSandboxURL    = "https://openapi-sandbox.keeta.com"
)

// NewKeetaService 創建 Keeta 服務
func NewKeetaService(config IntegrationConfig) *KeetaService {
	baseURL := KeetaProductionURL
	if config.Sandbox {
		baseURL = KeetaSandboxURL
	}

	// Allow overriding BaseURL from ExtraSettings (for testing)
	if url, ok := config.ExtraSettings["base_url"].(string); ok && url != "" {
		baseURL = url
	}

	return &KeetaService{
		Config:  config,
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest 執行 HTTP 請求
func (s *KeetaService) doRequest(method, path string, body interface{}) ([]byte, error) {
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

	// Keeta 使用 API Key + Secret 簽名
	timestamp := time.Now().Unix()
	signature := s.generateSignature(path, timestamp)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.Config.APIKey)
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Merchant-Id", s.Config.MerchantID)

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

// generateSignature 生成 API 簽名
func (s *KeetaService) generateSignature(path string, timestamp int64) string {
	payload := fmt.Sprintf("%s%d%s", s.Config.APIKey, timestamp, path)
	return GenerateHMACSignature(s.Config.APISecret, []byte(payload))
}

// TestConnection 測試連接
func (s *KeetaService) TestConnection() error {
	_, err := s.doRequest("GET", "/api/v1/merchant/info", nil)
	return err
}

// GetOrders 獲取訂單列表
func (s *KeetaService) GetOrders(since time.Time, limit int) ([]Order, error) {
	path := fmt.Sprintf("/api/v1/orders?start_time=%d&limit=%d", since.Unix(), limit)

	respBody, err := s.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Code    int                      `json:"code"`
		Message string                   `json:"message"`
		Data    []map[string]interface{} `json:"data"`
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
func (s *KeetaService) GetOrder(orderID string) (*Order, error) {
	path := fmt.Sprintf("/api/v1/orders/%s", orderID)

	respBody, err := s.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析響應失敗: %w", err)
	}

	order := s.parseOrder(resp.Data)
	return &order, nil
}

// AcceptOrder 接受訂單
func (s *KeetaService) AcceptOrder(orderID string) error {
	path := fmt.Sprintf("/api/v1/orders/%s/confirm", orderID)
	_, err := s.doRequest("POST", path, nil)
	return err
}

// RejectOrder 拒絕訂單
func (s *KeetaService) RejectOrder(orderID string, reason string) error {
	path := fmt.Sprintf("/api/v1/orders/%s/cancel", orderID)
	body := map[string]string{"cancel_reason": reason}
	_, err := s.doRequest("POST", path, body)
	return err
}

// MarkPreparing 標記為準備中
func (s *KeetaService) MarkPreparing(orderID string) error {
	path := fmt.Sprintf("/api/v1/orders/%s/status", orderID)
	body := map[string]string{"status": "preparing"}
	_, err := s.doRequest("PUT", path, body)
	return err
}

// MarkReadyForPickup 標記為可取餐
func (s *KeetaService) MarkReadyForPickup(orderID string) error {
	path := fmt.Sprintf("/api/v1/orders/%s/status", orderID)
	body := map[string]string{"status": "ready_for_pickup"}
	_, err := s.doRequest("PUT", path, body)
	return err
}

// SyncMenu 同步菜單
func (s *KeetaService) SyncMenu(products []MenuProduct) error {
	path := "/api/v1/menu/batch-update"
	_, err := s.doRequest("POST", path, map[string]interface{}{
		"items": products,
	})
	return err
}

// UpdateItemAvailability 更新商品可用狀態
func (s *KeetaService) UpdateItemAvailability(itemID string, available bool) error {
	path := fmt.Sprintf("/api/v1/menu/items/%s", itemID)
	status := "available"
	if !available {
		status = "unavailable"
	}
	_, err := s.doRequest("PUT", path, map[string]string{
		"status": status,
	})
	return err
}

// ValidateWebhook 驗證 Webhook 簽名
func (s *KeetaService) ValidateWebhook(signature string, payload []byte) bool {
	return ValidateHMACSignature(s.Config.WebhookSecret, signature, payload)
}

// ParseWebhookEvent 解析 Webhook 事件
func (s *KeetaService) ParseWebhookEvent(payload []byte) (*WebhookEvent, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("解析 webhook 失敗: %w", err)
	}

	event := &WebhookEvent{
		Platform:   PlatformKeeta,
		EventType:  getString(data, "event"),
		EventID:    getString(data, "event_id"),
		OrderID:    getString(data, "order_id"),
		RawPayload: payload,
		ParsedData: data,
		ReceivedAt: time.Now(),
	}

	// 映射狀態
	event.Status = s.mapStatus(getString(data, "order_status"))

	return event, nil
}

// RefreshToken 刷新 Token（Keeta 使用 API Key 認證，無需刷新）
func (s *KeetaService) RefreshToken() (string, string, *time.Time, error) {
	// Keeta 使用 API Key 認證，無需刷新 Token
	return "", "", nil, nil
}

// parseOrder 解析訂單數據
func (s *KeetaService) parseOrder(data map[string]interface{}) Order {
	order := Order{
		PlatformOrderID:     getString(data, "order_id"),
		PlatformOrderNumber: getString(data, "order_no"),
		Platform:            PlatformKeeta,
		PlatformStatus:      getString(data, "status"),
		CustomerName:        getString(data, "recipient_name"),
		CustomerPhone:       getString(data, "recipient_phone"),
		CustomerAddress:     getString(data, "delivery_address"),
		CustomerNotes:       getString(data, "remark"),
		DeliveryType:        getString(data, "order_type"),
		Subtotal:            getFloat(data, "food_amount"),
		DeliveryFee:         getFloat(data, "delivery_fee"),
		PlatformFee:         getFloat(data, "service_fee"),
		DiscountAmount:      getFloat(data, "discount_amount"),
		TotalAmount:         getFloat(data, "total_amount"),
		Currency:            "HKD", // Keeta 主要在香港運營
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
					PlatformItemID: getString(itemMap, "item_id"),
					Name:           getString(itemMap, "item_name"),
					Quantity:       getInt(itemMap, "quantity"),
					UnitPrice:      getFloat(itemMap, "price"),
					TotalPrice:     getFloat(itemMap, "total_price"),
					Notes:          getString(itemMap, "remark"),
				}
				order.Items = append(order.Items, orderItem)
			}
		}
	}

	// 騎手信息
	if rider, ok := data["rider_info"].(map[string]interface{}); ok {
		order.RiderName = getString(rider, "rider_name")
		order.RiderPhone = getString(rider, "rider_phone")
		order.RiderTrackingURL = getString(rider, "tracking_url")
	}

	return order
}

// mapStatus 映射平台狀態到統一狀態
func (s *KeetaService) mapStatus(platformStatus string) OrderStatus {
	switch platformStatus {
	case "new", "pending", "待確認":
		return OrderStatusPending
	case "confirmed", "已確認":
		return OrderStatusConfirmed
	case "preparing", "製作中":
		return OrderStatusPreparing
	case "ready", "待取餐":
		return OrderStatusReadyForPickup
	case "picked", "配送中":
		return OrderStatusPickedUp
	case "completed", "delivered", "已完成":
		return OrderStatusDelivered
	case "cancelled", "已取消":
		return OrderStatusCancelled
	case "failed", "異常":
		return OrderStatusFailed
	default:
		return OrderStatusPending
	}
}
