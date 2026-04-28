package delivery

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Platform 外賣平台類型
type Platform string

const (
	PlatformFoodpanda Platform = "foodpanda"
	PlatformKeeta     Platform = "keeta"
	PlatformDeliveroo Platform = "deliveroo"
)

// OrderStatus 訂單狀態
type OrderStatus string

const (
	OrderStatusPending        OrderStatus = "pending"
	OrderStatusConfirmed      OrderStatus = "confirmed"
	OrderStatusPreparing      OrderStatus = "preparing"
	OrderStatusReadyForPickup OrderStatus = "ready_for_pickup"
	OrderStatusPickedUp       OrderStatus = "picked_up"
	OrderStatusDelivered      OrderStatus = "delivered"
	OrderStatusCancelled      OrderStatus = "cancelled"
	OrderStatusFailed         OrderStatus = "failed"
)

// IntegrationConfig 整合配置
type IntegrationConfig struct {
	Platform      Platform
	MerchantID    string
	APIKey        string
	APISecret     string
	AccessToken   string
	RefreshToken  string
	WebhookSecret string
	Sandbox       bool // 是否使用沙箱環境
	ExtraSettings map[string]interface{}
}

// Order 統一訂單格式
type Order struct {
	ID                  string
	PlatformOrderID     string
	PlatformOrderNumber string
	Platform            Platform
	Status              OrderStatus
	PlatformStatus      string

	// 客戶信息
	CustomerName    string
	CustomerPhone   string
	CustomerAddress string
	CustomerNotes   string

	// 配送信息
	DeliveryType          string // delivery, pickup, dine_in
	EstimatedPickupTime   *time.Time
	EstimatedDeliveryTime *time.Time

	// 騎手信息
	RiderName        string
	RiderPhone       string
	RiderTrackingURL string

	// 金額
	Subtotal       float64
	DeliveryFee    float64
	PlatformFee    float64
	DiscountAmount float64
	TotalAmount    float64
	Currency       string

	// 訂單項目
	Items []OrderItem

	// 原始數據
	RawData map[string]interface{}

	// 時間戳
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OrderItem 訂單項目
type OrderItem struct {
	PlatformItemID string          `json:"platform_item_id"`
	ProductID      string          `json:"product_id,omitempty"`
	Name           string          `json:"name"`
	Quantity       int             `json:"quantity"`
	UnitPrice      float64         `json:"unit_price"`
	TotalPrice     float64         `json:"total_price"`
	Notes          string          `json:"notes,omitempty"`
	Modifiers      []OrderModifier `json:"modifiers,omitempty"`
}

// OrderModifier 訂單修改項（配料、加料等）
type OrderModifier struct {
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

// WebhookEvent Webhook 事件
type WebhookEvent struct {
	Platform   Platform
	EventType  string
	EventID    string
	OrderID    string
	Status     OrderStatus
	RawPayload []byte
	ParsedData map[string]interface{}
	ReceivedAt time.Time
}

// PlatformService 平台服務介面
type PlatformService interface {
	// 連接測試
	TestConnection() error

	// 訂單操作
	GetOrders(since time.Time, limit int) ([]Order, error)
	GetOrder(orderID string) (*Order, error)
	AcceptOrder(orderID string) error
	RejectOrder(orderID string, reason string) error
	MarkPreparing(orderID string) error
	MarkReadyForPickup(orderID string) error

	// 菜單同步
	SyncMenu(products []MenuProduct) error
	UpdateItemAvailability(itemID string, available bool) error

	// Webhook 驗證
	ValidateWebhook(signature string, payload []byte) bool
	ParseWebhookEvent(payload []byte) (*WebhookEvent, error)

	// Token 刷新
	RefreshToken() (string, string, *time.Time, error)
}

// MenuProduct 菜單產品（用於同步）
type MenuProduct struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	ImageURL    string  `json:"image_url"`
	Category    string  `json:"category"`
	Available   bool    `json:"available"`
}

// IntegrationService 整合服務
type IntegrationService struct {
	TenantID uuid.UUID
	Config   IntegrationConfig
	client   *http.Client
}

// NewIntegrationService 創建整合服務
func NewIntegrationService(tenantID uuid.UUID, config IntegrationConfig) *IntegrationService {
	return &IntegrationService{
		TenantID: tenantID,
		Config:   config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetPlatformService 獲取特定平台的服務
func (s *IntegrationService) GetPlatformService() (PlatformService, error) {
	switch s.Config.Platform {
	case PlatformFoodpanda:
		return NewFoodpandaService(s.Config), nil
	case PlatformKeeta:
		return NewKeetaService(s.Config), nil
	case PlatformDeliveroo:
		return NewDeliverooService(s.Config), nil
	default:
		return nil, fmt.Errorf("不支援的平台: %s", s.Config.Platform)
	}
}

// GenerateHMACSignature 生成 HMAC 簽名
func GenerateHMACSignature(secret string, payload []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateHMACSignature 驗證 HMAC 簽名
func ValidateHMACSignature(secret, signature string, payload []byte) bool {
	expected := GenerateHMACSignature(secret, payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ParseJSONBody 解析 JSON 響應
func ParseJSONBody(body io.Reader, v interface{}) error {
	decoder := json.NewDecoder(body)
	return decoder.Decode(v)
}
