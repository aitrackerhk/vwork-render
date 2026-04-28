package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RentalOrder 出租單
type RentalOrder struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_rental_orders_tenant_order_number" json:"tenant_id"`
	Tenant           Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	OrderNumber      string     `gorm:"type:varchar(100);not null;uniqueIndex:idx_rental_orders_tenant_order_number" json:"order_number"`
	CustomerID       *uuid.UUID `gorm:"type:uuid" json:"customer_id"`
	Customer         Customer   `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	RentalDate       time.Time  `gorm:"type:date;not null" json:"rental_date"`
	Status           string     `gorm:"type:varchar(50);not null;default:'draft'" json:"status"`
	TotalAmount      float64    `gorm:"type:decimal(15,2);default:0" json:"total_amount"`
	CouponID         *uuid.UUID `gorm:"type:uuid" json:"coupon_id,omitempty"`
	Coupon           *Coupon    `gorm:"foreignKey:CouponID" json:"coupon,omitempty"`
	PointsUsed       int        `gorm:"default:0" json:"points_used"`
	PointsEarned     int        `gorm:"default:0" json:"points_earned"`
	PointsDiscount   float64    `gorm:"type:decimal(18,2);default:0.00" json:"points_discount"`
	CouponDiscount   float64    `gorm:"type:decimal(18,2);default:0.00" json:"coupon_discount"`
	ReferralCode     string     `gorm:"type:varchar(50)" json:"referral_code,omitempty"`
	ContactName      string     `gorm:"type:varchar(255)" json:"contact_name,omitempty"`
	ContactEmail     string     `gorm:"type:varchar(255)" json:"contact_email,omitempty"`
	ContactPhone     string     `gorm:"type:varchar(50)" json:"contact_phone,omitempty"`
	ContactAddress   string     `gorm:"type:text" json:"contact_address,omitempty"`
	SalespersonID    *uuid.UUID `gorm:"type:uuid" json:"salesperson_id,omitempty"`
	Salesperson      *User      `gorm:"foreignKey:SalespersonID" json:"salesperson,omitempty"`
	StoreID          *uuid.UUID `gorm:"type:uuid" json:"store_id,omitempty"`
	Store            *Store     `gorm:"foreignKey:StoreID" json:"store,omitempty"`
	CommissionAmount float64    `gorm:"type:decimal(15,2);default:0" json:"commission_amount"`
	Notes            string     `gorm:"type:text" json:"notes"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CreatedBy        *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy        *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	TrashedAt        *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"`
	ExtraFields      JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`

	// 關聯
	RentalOrderItems []RentalOrderItem  `gorm:"foreignKey:RentalOrderID" json:"rental_order_items,omitempty"`
	Labels           []RentalOrderLabel `gorm:"many2many:rental_order_label_relations;foreignKey:ID;joinForeignKey:RentalOrderID;References:ID;joinReferences:LabelID" json:"labels,omitempty"`
	Appointments     []Appointment      `gorm:"foreignKey:RentalOrderID" json:"appointments,omitempty"`
}

func (ro *RentalOrder) BeforeCreate(tx *gorm.DB) error {
	if ro.ID == uuid.Nil {
		ro.ID = uuid.New()
	}
	return nil
}

func (RentalOrder) TableName() string {
	return "rental_orders"
}

// RentalOrderItem 出租單明細（資源明細：店舖/房間/設備/車輛）
type RentalOrderItem struct {
	ID            uuid.UUID   `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID   `gorm:"type:uuid;not null;index" json:"tenant_id"`
	RentalOrderID uuid.UUID   `gorm:"type:uuid;not null;index" json:"rental_order_id"`
	RentalOrder   RentalOrder `gorm:"foreignKey:RentalOrderID" json:"rental_order,omitempty"`
	ResourceType  string      `gorm:"type:varchar(50);not null" json:"resource_type"` // store, room, equipment, vehicle
	ResourceID    *uuid.UUID  `gorm:"type:uuid" json:"resource_id"`                   // 對應資源的 ID
	ResourceName  string      `gorm:"type:varchar(255)" json:"resource_name"`         // 冗餘存儲資源名稱
	StaffID       *uuid.UUID  `gorm:"type:uuid" json:"staff_id,omitempty"`
	Staff         *User       `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
	Quantity      float64     `gorm:"type:decimal(10,2);not null" json:"quantity"`
	UnitPrice     float64     `gorm:"type:decimal(15,2);not null" json:"unit_price"`
	TotalPrice    float64     `gorm:"type:decimal(15,2);not null" json:"total_price"`
	Notes         string      `gorm:"type:text" json:"notes"`
	CreatedAt     time.Time   `json:"created_at"`
	TrashedAt     *time.Time  `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"`
	ExtraFields   JSONB       `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
}

func (roi *RentalOrderItem) BeforeCreate(tx *gorm.DB) error {
	if roi.ID == uuid.Nil {
		roi.ID = uuid.New()
	}
	return nil
}

func (RentalOrderItem) TableName() string {
	return "rental_order_items"
}

// RentalOrderLabel 出租單標籤
type RentalOrderLabel struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant    Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name      string     `gorm:"type:varchar(255);not null" json:"name"`
	Color     string     `gorm:"type:varchar(7);not null;default:'#007bff'" json:"color"`
	Status    string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	TrashedAt *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"`
}

func (RentalOrderLabel) TableName() string {
	return "rental_order_labels"
}

func (rol *RentalOrderLabel) BeforeCreate(tx *gorm.DB) error {
	if rol.ID == uuid.Nil {
		rol.ID = uuid.New()
	}
	return nil
}
