package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DeliveryPlatform 外賣平台類型
type DeliveryPlatform string

const (
	DeliveryPlatformFoodpanda DeliveryPlatform = "foodpanda"
	DeliveryPlatformKeeta     DeliveryPlatform = "keeta"
	DeliveryPlatformDeliveroo DeliveryPlatform = "deliveroo"
)

// DeliveryOrderStatus 外賣訂單狀態
type DeliveryOrderStatus string

const (
	DeliveryOrderStatusPending        DeliveryOrderStatus = "pending"
	DeliveryOrderStatusConfirmed      DeliveryOrderStatus = "confirmed"
	DeliveryOrderStatusPreparing      DeliveryOrderStatus = "preparing"
	DeliveryOrderStatusReadyForPickup DeliveryOrderStatus = "ready_for_pickup"
	DeliveryOrderStatusPickedUp       DeliveryOrderStatus = "picked_up"
	DeliveryOrderStatusDelivered      DeliveryOrderStatus = "delivered"
	DeliveryOrderStatusCancelled      DeliveryOrderStatus = "cancelled"
	DeliveryOrderStatusFailed         DeliveryOrderStatus = "failed"
)

// DeliveryIntegration 外賣平台整合設定
type DeliveryIntegration struct {
	ID       uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	StoreID  *uuid.UUID `gorm:"type:uuid" json:"store_id"`
	Store    *Store     `gorm:"foreignKey:StoreID" json:"store,omitempty"`

	// 平台類型
	Platform DeliveryPlatform `gorm:"type:varchar(50);not null" json:"platform"`

	// 商戶信息
	MerchantID   string `gorm:"type:varchar(255)" json:"merchant_id"`
	MerchantName string `gorm:"type:varchar(255)" json:"merchant_name"`

	// API 認證
	APIKey         string     `gorm:"type:text" json:"-"` // 不輸出到 JSON
	APISecret      string     `gorm:"type:text" json:"-"`
	AccessToken    string     `gorm:"type:text" json:"-"`
	RefreshToken   string     `gorm:"type:text" json:"-"`
	TokenExpiresAt *time.Time `gorm:"type:timestamp" json:"token_expires_at,omitempty"`

	// Webhook 設定
	WebhookSecret string `gorm:"type:varchar(255)" json:"-"`
	WebhookURL    string `gorm:"type:varchar(500)" json:"webhook_url"`

	// 狀態
	IsEnabled   bool       `gorm:"default:true" json:"is_enabled"`
	IsConnected bool       `gorm:"default:false" json:"is_connected"`
	LastSyncAt  *time.Time `gorm:"type:timestamp" json:"last_sync_at,omitempty"`
	LastError   string     `gorm:"type:text" json:"last_error,omitempty"`

	// 設定選項
	Settings JSONB `gorm:"type:jsonb;default:'{}'" json:"settings"`

	// 審計字段
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	CreatedBy *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
}

func (d *DeliveryIntegration) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

func (DeliveryIntegration) TableName() string {
	return "delivery_integrations"
}

// DeliveryOrderDetail 外賣訂單補充資訊（主體數據在 orders 表）
// 一對一關聯：orders.id = delivery_order_details.order_id
type DeliveryOrderDetail struct {
	ID            uuid.UUID            `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OrderID       uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex" json:"order_id"`
	IntegrationID *uuid.UUID           `gorm:"type:uuid" json:"integration_id"`
	Integration   *DeliveryIntegration `gorm:"foreignKey:IntegrationID" json:"integration,omitempty"`

	// 平台資訊（冗餘存放方便查詢）
	Platform            DeliveryPlatform `gorm:"type:varchar(50);not null" json:"platform"`
	PlatformOrderID     string           `gorm:"type:varchar(255);not null" json:"platform_order_id"`
	PlatformOrderNumber string           `gorm:"type:varchar(100)" json:"platform_order_number"`
	PlatformStatus      string           `gorm:"type:varchar(100)" json:"platform_status"`

	// 配送資訊
	DeliveryType          string     `gorm:"type:varchar(50);default:'delivery'" json:"delivery_type"` // delivery, pickup, dine_in
	EstimatedPickupTime   *time.Time `gorm:"type:timestamp" json:"estimated_pickup_time,omitempty"`
	EstimatedDeliveryTime *time.Time `gorm:"type:timestamp" json:"estimated_delivery_time,omitempty"`
	ActualPickupTime      *time.Time `gorm:"type:timestamp" json:"actual_pickup_time,omitempty"`
	ActualDeliveryTime    *time.Time `gorm:"type:timestamp" json:"actual_delivery_time,omitempty"`

	// 騎手資訊
	RiderName        string `gorm:"type:varchar(255)" json:"rider_name"`
	RiderPhone       string `gorm:"type:varchar(100)" json:"rider_phone"`
	RiderTrackingURL string `gorm:"type:text" json:"rider_tracking_url"`

	// 外賣平台費用明細
	PlatformFee      float64 `gorm:"type:decimal(15,2);default:0" json:"platform_fee"`
	DeliveryFee      float64 `gorm:"type:decimal(15,2);default:0" json:"delivery_fee"`
	PlatformDiscount float64 `gorm:"type:decimal(15,2);default:0" json:"platform_discount"`

	// 原始數據（完整保留平台返回的 JSON）
	RawData JSONB `gorm:"type:jsonb;default:'{}'" json:"raw_data,omitempty"`

	// 取消資訊
	CancelledAt  *time.Time `gorm:"type:timestamp" json:"cancelled_at,omitempty"`
	CancelReason string     `gorm:"type:text" json:"cancel_reason,omitempty"`
	CancelledBy  string     `gorm:"type:varchar(50)" json:"cancelled_by,omitempty"` // platform, merchant, customer

	// 審計
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ConfirmedAt *time.Time `gorm:"type:timestamp" json:"confirmed_at,omitempty"`
}

func (d *DeliveryOrderDetail) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

func (DeliveryOrderDetail) TableName() string {
	return "delivery_order_details"
}

// DeliveryOrderItem 外賣訂單項目（用於 JSONB 解析）
type DeliveryOrderItem struct {
	PlatformItemID string  `json:"platform_item_id"`
	ProductID      string  `json:"product_id,omitempty"`
	Name           string  `json:"name"`
	Quantity       int     `json:"quantity"`
	UnitPrice      float64 `json:"unit_price"`
	TotalPrice     float64 `json:"total_price"`
	Notes          string  `json:"notes,omitempty"`
	Modifiers      []struct {
		Name     string  `json:"name"`
		Price    float64 `json:"price"`
		Quantity int     `json:"quantity"`
	} `json:"modifiers,omitempty"`
}

// DeliveryOrderStatusHistory 外賣訂單狀態歷史
type DeliveryOrderStatusHistory struct {
	ID      uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OrderID uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"` // 關聯到 orders 表

	Status         string `gorm:"type:varchar(50);not null" json:"status"`
	PlatformStatus string `gorm:"type:varchar(100)" json:"platform_status"`
	Notes          string `gorm:"type:text" json:"notes"`
	RawEvent       JSONB  `gorm:"type:jsonb;default:'{}'" json:"raw_event,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	CreatedBy *uuid.UUID `gorm:"type:uuid" json:"created_by"`
}

func (d *DeliveryOrderStatusHistory) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

func (DeliveryOrderStatusHistory) TableName() string {
	return "delivery_order_status_history"
}

// DeliveryProductMapping 外賣平台商品映射表
type DeliveryProductMapping struct {
	ID            uuid.UUID            `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID            `gorm:"type:uuid;not null;index" json:"tenant_id"`
	IntegrationID *uuid.UUID           `gorm:"type:uuid" json:"integration_id"`
	Integration   *DeliveryIntegration `gorm:"foreignKey:IntegrationID" json:"integration,omitempty"`

	// 平台商品資訊
	Platform         DeliveryPlatform `gorm:"type:varchar(50);not null" json:"platform"`
	PlatformItemID   string           `gorm:"type:varchar(255);not null" json:"platform_item_id"`
	PlatformItemName string           `gorm:"type:varchar(500)" json:"platform_item_name"`
	PlatformCategory string           `gorm:"type:varchar(255)" json:"platform_category"`

	// 映射到 vWork 產品
	ProductID *uuid.UUID `gorm:"type:uuid" json:"product_id"`
	Product   *Product   `gorm:"foreignKey:ProductID" json:"product,omitempty"`

	// 價格差異
	PlatformPrice   float64 `gorm:"type:decimal(15,2)" json:"platform_price"`
	PriceDifference float64 `gorm:"type:decimal(15,2);default:0" json:"price_difference"`

	// 庫存同步設定
	SyncStock     bool     `gorm:"default:false" json:"sync_stock"`
	SyncInventory bool     `gorm:"default:false" json:"sync_inventory"` // 是否扣減內部庫存
	StockBuffer   int      `gorm:"default:0" json:"stock_buffer"`
	QuantityRatio *float64 `gorm:"type:decimal(10,4);default:1" json:"quantity_ratio"` // 數量比例（例如平台1份=內部2份）

	// 狀態
	IsActive     bool       `gorm:"default:true" json:"is_active"`
	LastSyncedAt *time.Time `gorm:"type:timestamp" json:"last_synced_at,omitempty"`

	// 審計
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (d *DeliveryProductMapping) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

func (DeliveryProductMapping) TableName() string {
	return "delivery_product_mappings"
}
