package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PaymentMethod 付款方式
type PaymentMethod struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant    Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	Code      string    `gorm:"type:varchar(50);not null" json:"code"`
	IsDefault bool      `gorm:"default:false" json:"is_default"`
	// IsDefaultExpense: 系統預設「支出」付款方法（相關支出/退款/採購支出等）
	IsDefaultExpense bool `gorm:"default:false" json:"is_default_expense"`
	// IsOnlinePayment: 網店付款方式（是否在 checkout 元件中顯示）
	IsOnlinePayment bool       `gorm:"default:false" json:"is_online_payment"`
	Status          string     `gorm:"type:varchar(50);default:'active'" json:"status"`
	ExtraFields     JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	TrashedAt       *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (PaymentMethod) TableName() string {
	return "payment_methods"
}

// ShippingMethod 運送方式
type ShippingMethod struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant           Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name             string     `gorm:"type:varchar(255);not null" json:"name"`
	Code             string     `gorm:"type:varchar(50);not null" json:"code"`
	RequiresShipping bool       `gorm:"default:false" json:"requires_shipping"` // 是否需要送貨
	IsDefault        bool       `gorm:"default:false" json:"is_default"`
	Status           string     `gorm:"type:varchar(50);default:'active'" json:"status"`
	ExtraFields      JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	TrashedAt        *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (ShippingMethod) TableName() string {
	return "shipping_methods"
}

// LogisticsCompany 物流公司
type LogisticsCompany struct {
	ID              uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant          Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name            string     `gorm:"type:varchar(255);not null" json:"name"`
	Code            string     `gorm:"type:varchar(50)" json:"code"`
	IntegrationType string     `gorm:"type:varchar(50);default:'none'" json:"integration_type"` // 配送連接類型: none, sfexpress, lalamove
	BaseFee         float64    `gorm:"type:decimal(15,2);default:0" json:"base_fee"`            // 預設定額
	PerItemFee      float64    `gorm:"type:decimal(15,2);default:0" json:"per_item_fee"`        // 件價
	PerWeightFee    float64    `gorm:"type:decimal(15,2);default:0" json:"per_weight_fee"`      // 重量價（每公斤）
	PerAreaFee      float64    `gorm:"type:decimal(15,2);default:0" json:"per_area_fee"`        // 面積價（每平方米）
	Status          string     `gorm:"type:varchar(50);default:'active'" json:"status"`
	IsDefault       bool       `gorm:"default:false" json:"is_default"` // 是否為系統預設物流公司
	ExtraFields     JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	TrashedAt       *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (LogisticsCompany) TableName() string {
	return "logistics_companies"
}

// BeforeCreate 創建前設置 UUID
func (p *PaymentMethod) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

func (s *ShippingMethod) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (l *LogisticsCompany) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}

// BankAccount 銀行賬戶
type BankAccount struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID           uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant             Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name               string     `gorm:"type:varchar(255);not null" json:"name"`           // 賬戶名稱（如：中國銀行主賬戶）
	BankName           string     `gorm:"type:varchar(255);not null" json:"bank_name"`      // 銀行名稱（如：中國銀行）
	AccountNumber      string     `gorm:"type:varchar(100);not null" json:"account_number"` // 賬戶號碼
	AccountHolder      string     `gorm:"type:varchar(255)" json:"account_holder"`          // 戶名
	Currency           string     `gorm:"type:varchar(10);default:'HKD'" json:"currency"`   // 幣種（HKD, USD, CNY等）
	IsDefault          bool       `gorm:"default:false" json:"is_default"`                  // 是否為默認賬戶（保留以兼容舊數據）
	IsDefaultReceiving bool       `gorm:"default:false" json:"is_default_receiving"`        // 是否為系統預設收款帳號
	IsDefaultPayment   bool       `gorm:"default:false" json:"is_default_payment"`          // 是否為系統預設付款帳號
	Status             string     `gorm:"type:varchar(50);default:'active'" json:"status"`  // active, inactive
	Notes              string     `gorm:"type:text" json:"notes"`                           // 備註
	ExtraFields        JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	TrashedAt          *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (BankAccount) TableName() string {
	return "bank_accounts"
}

func (b *BankAccount) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

// InventorySettings 出入庫設定
type InventorySettings struct {
	ID                         uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID                   uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"tenant_id"`
	Tenant                     Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	RequiresOutbound           bool       `gorm:"default:true" json:"requires_outbound"`               // 訂單是否需要出庫
	RequiresInbound            bool       `gorm:"default:true" json:"requires_inbound"`                // 採購/退款是否需要入庫
	RequiresItemByItemOutbound bool       `gorm:"default:false" json:"requires_item_by_item_outbound"` // 是否需要續件出庫
	RequiresItemByItemInbound  bool       `gorm:"default:false" json:"requires_item_by_item_inbound"`  // 是否需要續件入庫
	AutoCompleteIfNoNeed       bool       `gorm:"default:true" json:"auto_complete_if_no_need"`        // 如果不需要出入庫，自動完成狀態
	ExtraFields                JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
	CreatedBy                  *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy                  *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
}

func (InventorySettings) TableName() string {
	return "inventory_settings"
}

func (s *InventorySettings) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}
