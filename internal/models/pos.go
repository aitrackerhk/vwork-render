package models

import (
	"time"

	"github.com/google/uuid"
)

// POSSale POS銷售
type POSSale struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_pos_sales_tenant_sale_number" json:"tenant_id"`
	Tenant         Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	CustomerID     *uuid.UUID `gorm:"type:uuid" json:"customer_id,omitempty"`
	Customer       *Customer  `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	StaffID        *uuid.UUID `gorm:"type:uuid" json:"staff_id,omitempty"`
	Staff          *User      `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
	SaleNumber     string     `gorm:"type:varchar(100);uniqueIndex:idx_pos_sales_tenant_sale_number" json:"sale_number"`
	SaleDate       time.Time  `gorm:"type:timestamp;not null" json:"sale_date"`
	Subtotal       float64    `gorm:"type:decimal(18,2);default:0.00" json:"subtotal"`
	DiscountAmount float64    `gorm:"type:decimal(18,2);default:0.00" json:"discount_amount"`
	TaxAmount      float64    `gorm:"type:decimal(18,2);default:0.00" json:"tax_amount"`
	TotalAmount    float64    `gorm:"type:decimal(18,2);default:0.00" json:"total_amount"`
	Status         string     `gorm:"type:varchar(50);default:'completed'" json:"status"`
	Notes          string     `gorm:"type:text" json:"notes"`
	ExtraFields    JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (POSSale) TableName() string {
	return "pos_sales"
}

// POSSaleItem POS銷售明細
type POSSaleItem struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	POSSaleID      uuid.UUID  `gorm:"type:uuid;not null" json:"pos_sale_id"`
	POSSale        POSSale    `gorm:"foreignKey:POSSaleID" json:"pos_sale,omitempty"`
	ProductID      *uuid.UUID `gorm:"type:uuid" json:"product_id,omitempty"`
	Product        *Product   `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	ServiceID      *uuid.UUID `gorm:"type:uuid" json:"service_id,omitempty"`
	Service        *Service   `gorm:"foreignKey:ServiceID" json:"service,omitempty"`
	ItemName       string     `gorm:"type:varchar(255);not null" json:"item_name"`
	Quantity       int        `gorm:"not null" json:"quantity"`
	UnitPrice      float64    `gorm:"type:decimal(18,2);default:0.00" json:"unit_price"`
	DiscountAmount float64    `gorm:"type:decimal(18,2);default:0.00" json:"discount_amount"`
	TaxRate        float64    `gorm:"type:decimal(5,2);default:0.00" json:"tax_rate"`
	TotalAmount    float64    `gorm:"type:decimal(18,2);default:0.00" json:"total_amount"`
	ExtraFields    JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (POSSaleItem) TableName() string {
	return "pos_sale_items"
}

// POSPayment POS支付
type POSPayment struct {
	ID              uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	POSSaleID       uuid.UUID  `gorm:"type:uuid;not null" json:"pos_sale_id"`
	POSSale         POSSale    `gorm:"foreignKey:POSSaleID" json:"pos_sale,omitempty"`
	PaymentMethod   string     `gorm:"type:varchar(50);not null" json:"payment_method"`
	Amount          float64    `gorm:"type:decimal(18,2);not null" json:"amount"`
	CurrencyID      *uuid.UUID `gorm:"type:uuid" json:"currency_id,omitempty"`
	Currency        *Currency  `gorm:"foreignKey:CurrencyID" json:"currency,omitempty"`
	ExchangeRate    float64    `gorm:"type:decimal(18,6);default:1.0" json:"exchange_rate"`
	ReferenceNumber *string    `gorm:"type:varchar(255)" json:"reference_number,omitempty"`
	Status          string     `gorm:"type:varchar(50);default:'completed'" json:"status"`
	ExtraFields     JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (POSPayment) TableName() string {
	return "pos_payments"
}

// OrderReport 訂單報表
type OrderReport struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant      Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	ReportType  string    `gorm:"type:varchar(100);not null" json:"report_type"`
	ReportDate  time.Time `gorm:"type:date;not null" json:"report_date"`
	TotalOrders int       `gorm:"default:0" json:"total_orders"`
	TotalAmount float64   `gorm:"type:decimal(18,2);default:0.00" json:"total_amount"`
	TotalItems  int       `gorm:"default:0" json:"total_items"`
	ReportData  JSONB     `gorm:"type:jsonb;default:'{}'" json:"report_data,omitempty"`
	Status      string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (OrderReport) TableName() string {
	return "order_reports"
}
