package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Order 訂單模型
type Order struct {
	ID               uuid.UUID       `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID       `gorm:"type:uuid;not null;index" json:"tenant_id"`
	OrderNumber      string          `gorm:"type:varchar(100);not null" json:"order_number"`
	CustomerID       *uuid.UUID      `gorm:"type:uuid" json:"customer_id"`
	OrderDate        time.Time       `gorm:"type:date;not null" json:"order_date"`
	Status           string          `gorm:"type:varchar(50);not null;default:'draft'" json:"status"`
	TotalAmount      float64         `gorm:"type:decimal(15,2);default:0" json:"total_amount"`
	CouponID         *uuid.UUID      `gorm:"type:uuid" json:"coupon_id,omitempty"`
	PointsUsed       int             `gorm:"default:0" json:"points_used"`
	PointsEarned     int             `gorm:"default:0" json:"points_earned"`
	PointsDiscount   float64         `gorm:"type:decimal(18,2);default:0.00" json:"points_discount"`
	CouponDiscount   float64         `gorm:"type:decimal(18,2);default:0.00" json:"coupon_discount"`
	ReferralCode     string          `gorm:"type:varchar(50)" json:"referral_code,omitempty"`
	ContactName      string          `gorm:"type:varchar(255)" json:"contact_name,omitempty"`
	ContactEmail     string          `gorm:"type:varchar(255)" json:"contact_email,omitempty"`
	ContactPhone     string          `gorm:"type:varchar(50)" json:"contact_phone,omitempty"`
	ContactAddress   string          `gorm:"type:text" json:"contact_address,omitempty"`
	ShippingMethodID *uuid.UUID      `gorm:"type:uuid" json:"shipping_method_id,omitempty"`
	ShippingMethod   *ShippingMethod `gorm:"foreignKey:ShippingMethodID" json:"shipping_method,omitempty"`
	SalespersonID    *uuid.UUID      `gorm:"type:uuid" json:"salesperson_id,omitempty"` // 銷售員（員工）ID
	Salesperson      *User           `gorm:"foreignKey:SalespersonID" json:"salesperson,omitempty"`
	StoreID          *uuid.UUID      `gorm:"type:uuid" json:"store_id,omitempty"` // 所屬店舖ID
	Store            *Store          `gorm:"foreignKey:StoreID" json:"store,omitempty"`
	CommissionAmount float64         `gorm:"type:decimal(15,2);default:0" json:"commission_amount"` // 佣金金額
	Notes            string          `gorm:"type:text" json:"notes"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	CreatedBy        *uuid.UUID      `gorm:"type:uuid" json:"created_by"`
	UpdatedBy        *uuid.UUID      `gorm:"type:uuid" json:"updated_by"`
	// SourceType 訂單來源：erp / pos / webstore / delivery（真 DB 欄位）
	SourceType  string     `gorm:"type:varchar(20);not null;default:'erp'" json:"source_type"`
	TrashedAt   *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`

	// 外賣平台欄位（source_type='delivery' 時使用）
	DeliveryPlatform *string `gorm:"type:varchar(50)" json:"delivery_platform,omitempty"` // foodpanda, keeta, deliveroo
	PlatformOrderID  *string `gorm:"type:varchar(255)" json:"platform_order_id,omitempty"`

	// 關聯
	Customer            *Customer            `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	Coupon              *Coupon              `gorm:"foreignKey:CouponID" json:"coupon,omitempty"`
	OrderItems          []OrderItem          `gorm:"foreignKey:OrderID" json:"order_items,omitempty"`
	Labels              []OrderLabel         `gorm:"many2many:order_label_relations;foreignKey:ID;joinForeignKey:OrderID;References:ID;joinReferences:LabelID" json:"labels,omitempty"`
	DeliveryOrderDetail *DeliveryOrderDetail `gorm:"foreignKey:OrderID" json:"delivery_detail,omitempty"` // 外賣訂單補充資訊
}

// BeforeCreate 創建前設置 UUID
func (o *Order) BeforeCreate(tx *gorm.DB) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (Order) TableName() string {
	return "orders"
}

// OrderItem 訂單明細模型
type OrderItem struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	OrderID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"order_id"`
	ProductID   *uuid.UUID `gorm:"type:uuid" json:"product_id"` // 可為空（外賣訂單可能沒有對應產品）
	Quantity    float64    `gorm:"type:decimal(10,2);not null" json:"quantity"`
	UnitPrice   float64    `gorm:"type:decimal(15,2);not null" json:"unit_price"`
	TotalPrice  float64    `gorm:"type:decimal(15,2);not null" json:"total_price"`
	Notes       string     `gorm:"type:text" json:"notes"`
	CreatedAt   time.Time  `json:"created_at"`
	TrashedAt   *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`

	// 外賣平台商品資訊（product_id 為空時使用）
	PlatformItemID *string `gorm:"type:varchar(255)" json:"platform_item_id,omitempty"` // 外賣平台商品ID
	ItemName       *string `gorm:"type:varchar(500)" json:"item_name,omitempty"`        // 商品名稱
	ItemOptions    JSONB   `gorm:"type:jsonb;default:'[]'" json:"item_options"`         // 商品選項/加料

	// 關聯
	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (oi *OrderItem) BeforeCreate(tx *gorm.DB) error {
	if oi.ID == uuid.Nil {
		oi.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (OrderItem) TableName() string {
	return "order_items"
}
