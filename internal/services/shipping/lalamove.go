package shipping

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// LalamoveService Lalamove API 服務
type LalamoveService struct {
	APIKey      string
	APISecret   string
	Environment string // sandbox 或 production
	Market      string // HK, SG, TW, TH, VN, PH, MY, ID
}

// LalamoveConfig 從配置中構建服務
type LalamoveConfig struct {
	Enabled          bool   `json:"enabled"`
	Environment      string `json:"environment"`
	Market           string `json:"market"`
	APIKey           string `json:"api_key"`
	APISecret        string `json:"api_secret"`
	AutoQuote        bool   `json:"auto_quote"`
	AutoTrack        bool   `json:"auto_track"`
	ServiceType      string `json:"service_type"`
	RequireSignature bool   `json:"require_signature"`
	PurchaseService  bool   `json:"purchase_service"`
}

// LalamoveQuoteRequest 報價請求
type LalamoveQuoteRequest struct {
	ServiceType string            `json:"serviceType"` // MOTORCYCLE, CAR, VAN, TRUCK330, TRUCK550
	Stops       []LalamoveStop    `json:"stops"`
	Deliveries  []LalamoveDelivery `json:"deliveries"`
}

// LalamoveStop 站點信息
type LalamoveStop struct {
	Coordinates LalamoveCoordinates `json:"coordinates"`
	Address     string              `json:"address"`
}

// LalamoveCoordinates 坐標
type LalamoveCoordinates struct {
	Lat string `json:"lat"`
	Lng string `json:"lng"`
}

// LalamoveDelivery 配送詳情
type LalamoveDelivery struct {
	ToStop   int    `json:"toStop"`
	ToContact LalamoveContact `json:"toContact"`
	Remarks  string `json:"remarks"`
}

// LalamoveContact 聯繫人
type LalamoveContact struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// LalamoveQuoteResponse 報價響應
type LalamoveQuoteResponse struct {
	Success       bool    `json:"success"`
	Message       string  `json:"message"`
	QuotationID   string  `json:"quotation_id"`
	TotalFee      float64 `json:"total_fee"`
	TotalFeeCurrency string `json:"total_fee_currency"`
	ExpiresAt     string  `json:"expires_at"`
}

// LalamoveOrderRequest 創建訂單請求
type LalamoveOrderRequest struct {
	QuotationID string            `json:"quotationId"`
	Sender      LalamoveContact   `json:"sender"`
	Stops       []LalamoveStop    `json:"stops"`
	Deliveries  []LalamoveDelivery `json:"deliveries"`
	IsPODEnabled bool              `json:"isPODEnabled"` // 需要簽收
}

// LalamoveOrderResponse 創建訂單響應
type LalamoveOrderResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	OrderRef string `json:"order_ref"` // Lalamove 訂單號
	ShareLink string `json:"share_link"` // 追蹤連結
}

// LalamoveTrackResponse 追蹤響應
type LalamoveTrackResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Status    string `json:"status"`
	DriverName string `json:"driver_name"`
	DriverPhone string `json:"driver_phone"`
	VehiclePlate string `json:"vehicle_plate"`
}

// NewLalamoveService 創建 Lalamove 服務實例
func NewLalamoveService(config LalamoveConfig) *LalamoveService {
	return &LalamoveService{
		APIKey:      config.APIKey,
		APISecret:   config.APISecret,
		Environment: config.Environment,
		Market:      config.Market,
	}
}

// getBaseURL 獲取 API 基礎 URL
func (s *LalamoveService) getBaseURL() string {
	if s.Environment == "production" {
		return "https://rest.lalamove.com"
	}
	return "https://rest.sandbox.lalamove.com"
}

// generateSignature 生成 HMAC-SHA256 簽名
func (s *LalamoveService) generateSignature(timestamp int64, method, path, body string) string {
	// Lalamove 簽名格式: HMAC-SHA256(timestamp + method + path + body, api_secret)
	rawSignature := fmt.Sprintf("%d\r\n%s\r\n%s\r\n\r\n%s", timestamp, method, path, body)
	h := hmac.New(sha256.New, []byte(s.APISecret))
	h.Write([]byte(rawSignature))
	return hex.EncodeToString(h.Sum(nil))
}

// makeRequest 發送 API 請求
func (s *LalamoveService) makeRequest(method, path string, body interface{}) ([]byte, error) {
	timestamp := time.Now().UnixMilli()
	
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化請求失敗: %w", err)
		}
	}
	
	signature := s.generateSignature(timestamp, method, path, string(bodyBytes))
	
	req, err := http.NewRequest(method, s.getBaseURL()+path, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("創建請求失敗: %w", err)
	}
	
	// 設置請求頭
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("hmac %s:%d:%s", s.APIKey, timestamp, signature))
	req.Header.Set("Market", s.Market)
	req.Header.Set("X-LLM-Country", s.Market)
	req.Header.Set("X-Request-ID", strconv.FormatInt(timestamp, 10))
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("請求失敗: %w", err)
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("讀取響應失敗: %w", err)
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API 錯誤 %d: %s", resp.StatusCode, string(respBody))
	}
	
	return respBody, nil
}

// GetQuotation 獲取報價
func (s *LalamoveService) GetQuotation(req LalamoveQuoteRequest) (*LalamoveQuoteResponse, error) {
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"serviceType": req.ServiceType,
			"stops":       req.Stops,
			"deliveries":  req.Deliveries,
		},
	}
	
	respBody, err := s.makeRequest("POST", "/v3/quotations", body)
	if err != nil {
		return &LalamoveQuoteResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	
	// 解析響應
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &LalamoveQuoteResponse{
			Success: false,
			Message: "解析響應失敗: " + err.Error(),
		}, nil
	}
	
	// 提取數據
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return &LalamoveQuoteResponse{
			Success: false,
			Message: "響應格式錯誤",
		}, nil
	}
	
	quotationID, _ := data["quotationId"].(string)
	
	var totalFee float64
	if priceBreakdown, ok := data["priceBreakdown"].(map[string]interface{}); ok {
		if total, ok := priceBreakdown["total"].(string); ok {
			totalFee, _ = strconv.ParseFloat(total, 64)
		}
	}
	
	return &LalamoveQuoteResponse{
		Success:     true,
		Message:     "報價成功",
		QuotationID: quotationID,
		TotalFee:    totalFee,
	}, nil
}

// CreateOrder 創建訂單
func (s *LalamoveService) CreateOrder(req LalamoveOrderRequest) (*LalamoveOrderResponse, error) {
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"quotationId":  req.QuotationID,
			"sender":       req.Sender,
			"stops":        req.Stops,
			"deliveries":   req.Deliveries,
			"isPODEnabled": req.IsPODEnabled,
		},
	}
	
	respBody, err := s.makeRequest("POST", "/v3/orders", body)
	if err != nil {
		return &LalamoveOrderResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	
	// 解析響應
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &LalamoveOrderResponse{
			Success: false,
			Message: "解析響應失敗: " + err.Error(),
		}, nil
	}
	
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return &LalamoveOrderResponse{
			Success: false,
			Message: "響應格式錯誤",
		}, nil
	}
	
	orderRef, _ := data["orderRef"].(string)
	shareLink, _ := data["shareLink"].(string)
	
	return &LalamoveOrderResponse{
		Success:   true,
		Message:   "訂單創建成功",
		OrderRef:  orderRef,
		ShareLink: shareLink,
	}, nil
}

// GetOrderStatus 查詢訂單狀態
func (s *LalamoveService) GetOrderStatus(orderRef string) (*LalamoveTrackResponse, error) {
	respBody, err := s.makeRequest("GET", "/v3/orders/"+orderRef, nil)
	if err != nil {
		return &LalamoveTrackResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	
	// 解析響應
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &LalamoveTrackResponse{
			Success: false,
			Message: "解析響應失敗: " + err.Error(),
		}, nil
	}
	
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return &LalamoveTrackResponse{
			Success: false,
			Message: "響應格式錯誤",
		}, nil
	}
	
	status, _ := data["status"].(string)
	
	var driverName, driverPhone, vehiclePlate string
	if driver, ok := data["driver"].(map[string]interface{}); ok {
		driverName, _ = driver["name"].(string)
		driverPhone, _ = driver["phone"].(string)
		vehiclePlate, _ = driver["plateNumber"].(string)
	}
	
	return &LalamoveTrackResponse{
		Success:      true,
		Message:      "查詢成功",
		Status:       status,
		DriverName:   driverName,
		DriverPhone:  driverPhone,
		VehiclePlate: vehiclePlate,
	}, nil
}

// ParseLalamoveConfigFromJSON 從 JSON 配置解析 Lalamove 配置
func ParseLalamoveConfigFromJSON(data map[string]interface{}) *LalamoveConfig {
	if data == nil {
		return nil
	}

	llmData, ok := data["lalamove"].(map[string]interface{})
	if !ok {
		return nil
	}

	config := &LalamoveConfig{}

	if enabled, ok := llmData["enabled"].(bool); ok {
		config.Enabled = enabled
	}
	if env, ok := llmData["environment"].(string); ok {
		config.Environment = env
	}
	if market, ok := llmData["market"].(string); ok {
		config.Market = market
	}
	if key, ok := llmData["api_key"].(string); ok {
		config.APIKey = key
	}
	if secret, ok := llmData["api_secret"].(string); ok {
		config.APISecret = secret
	}
	if auto, ok := llmData["auto_quote"].(bool); ok {
		config.AutoQuote = auto
	}
	if track, ok := llmData["auto_track"].(bool); ok {
		config.AutoTrack = track
	}
	if st, ok := llmData["service_type"].(string); ok {
		config.ServiceType = st
	}
	if sig, ok := llmData["require_signature"].(bool); ok {
		config.RequireSignature = sig
	}
	if ps, ok := llmData["purchase_service"].(bool); ok {
		config.PurchaseService = ps
	}

	return config
}

// MapLalamoveStatusToShipmentStatus 將 Lalamove 狀態轉換為系統配送狀態
func MapLalamoveStatusToShipmentStatus(status string) string {
	switch status {
	case "PENDING", "ASSIGNING_DRIVER":
		return "pending"
	case "ON_GOING", "PICKED_UP":
		return "picked_up"
	case "IN_TRANSIT":
		return "in_transit"
	case "COMPLETED":
		return "delivered"
	case "CANCELED", "REJECTED":
		return "cancelled"
	case "EXPIRED":
		return "failed"
	default:
		return "in_transit"
	}
}
