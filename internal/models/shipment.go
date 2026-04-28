package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Shipment 配送記錄
// 可從出入貨記錄轉換，用於實際物流配送追蹤
type Shipment struct {
	ID                  uuid.UUID         `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID            uuid.UUID         `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant              Tenant            `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	ShipmentNumber      string            `gorm:"type:varchar(50);uniqueIndex" json:"shipment_number"`
	LogisticsCompanyID  *uuid.UUID        `gorm:"type:uuid" json:"logistics_company_id"`
	LogisticsCompany    *LogisticsCompany `gorm:"foreignKey:LogisticsCompanyID" json:"logistics_company,omitempty"`
	TrackingNumber      string            `gorm:"type:varchar(100)" json:"tracking_number"`
	OrderID             *uuid.UUID        `gorm:"type:uuid;index" json:"order_id"`
	Order               *Order            `gorm:"foreignKey:OrderID" json:"order,omitempty"`
	InventoryMovementID *uuid.UUID        `gorm:"type:uuid;index" json:"inventory_movement_id"` // 參考出入貨記錄 (訂單或採購單的 shipping_note ID)

	// 發件人信息
	SenderName    string `gorm:"type:varchar(100)" json:"sender_name"`
	SenderPhone   string `gorm:"type:varchar(50)" json:"sender_phone"`
	SenderAddress string `gorm:"type:text" json:"sender_address"`

	// 收件人信息
	RecipientName    string `gorm:"type:varchar(100)" json:"recipient_name"`
	RecipientPhone   string `gorm:"type:varchar(50)" json:"recipient_phone"`
	RecipientAddress string `gorm:"type:text" json:"recipient_address"`

	// 配送詳情
	Weight      float64 `gorm:"type:decimal(15,3);default:0" json:"weight"` // 重量（公斤）
	Dimensions  string  `gorm:"type:varchar(100)" json:"dimensions"`        // 尺寸（長x寬x高）
	ItemCount   int     `gorm:"default:1" json:"item_count"`                // 件數
	Description string  `gorm:"type:text" json:"description"`               // 配送內容描述

	// 費用
	ShippingFee  float64 `gorm:"type:decimal(15,2);default:0" json:"shipping_fee"`
	InsuranceFee float64 `gorm:"type:decimal(15,2);default:0" json:"insurance_fee"`
	TotalFee     float64 `gorm:"type:decimal(15,2);default:0" json:"total_fee"`

	// 狀態
	// pending: 待處理, picked_up: 已取件, in_transit: 運送中,
	// out_for_delivery: 派送中, delivered: 已送達,
	// failed: 配送失敗, returned: 已退回, cancelled: 已取消
	Status              string     `gorm:"type:varchar(50);default:'pending'" json:"status"`
	EstimatedDeliveryAt *time.Time `json:"estimated_delivery_at"`
	ActualDeliveryAt    *time.Time `json:"actual_delivery_at"`
	PickedUpAt          *time.Time `json:"picked_up_at"`

	Notes       string `gorm:"type:text" json:"notes"`
	ExtraFields JSONB  `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`

	CreatedBy     *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	CreatedByUser *User      `gorm:"foreignKey:CreatedBy" json:"created_by_user,omitempty"`
	UpdatedBy     *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	TrashedAt     *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Shipment) TableName() string {
	return "shipments"
}

func (s *Shipment) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// ShipmentStatusHistory 配送狀態歷史
type ShipmentStatusHistory struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	ShipmentID  uuid.UUID `gorm:"type:uuid;not null;index" json:"shipment_id"`
	Shipment    Shipment  `gorm:"foreignKey:ShipmentID" json:"shipment,omitempty"`
	Status      string    `gorm:"type:varchar(50);not null" json:"status"`
	Location    string    `gorm:"type:varchar(255)" json:"location"`
	Description string    `gorm:"type:text" json:"description"`
	OccurredAt  time.Time `json:"occurred_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (ShipmentStatusHistory) TableName() string {
	return "shipment_status_histories"
}

func (h *ShipmentStatusHistory) BeforeCreate(tx *gorm.DB) error {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	return nil
}
