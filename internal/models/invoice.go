package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Invoice 發票模型
type Invoice struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	InvoiceNumber string   `gorm:"type:varchar(100);not null" json:"invoice_number"`
	OrderID      *uuid.UUID `gorm:"type:uuid" json:"order_id"`
	CustomerID   *uuid.UUID `gorm:"type:uuid" json:"customer_id"`
	InvoiceDate  time.Time `gorm:"type:date;not null" json:"invoice_date"`
	DueDate      *time.Time `gorm:"type:date" json:"due_date"`
	Status       string    `gorm:"type:varchar(50);not null;default:'draft'" json:"status"`
	Subtotal     float64   `gorm:"type:decimal(15,2);default:0" json:"subtotal"`
	TaxAmount    float64   `gorm:"type:decimal(15,2);default:0" json:"tax_amount"`
	TotalAmount  float64   `gorm:"type:decimal(15,2);default:0" json:"total_amount"`
	PaidAmount   float64   `gorm:"type:decimal(15,2);default:0" json:"paid_amount"`
	Notes        string    `gorm:"type:text" json:"notes"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedBy    *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy    *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	ExtraFields  JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	
	// 關聯
	Customer     *Customer  `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	Order        *Order     `gorm:"foreignKey:OrderID" json:"order,omitempty"`
	Payments     []Payment  `gorm:"foreignKey:InvoiceID" json:"payments,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (i *Invoice) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (Invoice) TableName() string {
	return "invoices"
}

// Payment 支付記錄模型
type Payment struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	InvoiceID     *uuid.UUID `gorm:"type:uuid" json:"invoice_id"`
	PaymentDate   time.Time `gorm:"type:date;not null" json:"payment_date"`
	Amount        float64   `gorm:"type:decimal(15,2);not null" json:"amount"`
	PaymentMethod string    `gorm:"type:varchar(50)" json:"payment_method"`
	ReferenceNumber string  `gorm:"type:varchar(100)" json:"reference_number"`
	Notes         string    `gorm:"type:text" json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	ExtraFields   JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	
	// 關聯
	Invoice      *Invoice   `gorm:"foreignKey:InvoiceID" json:"invoice,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (p *Payment) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (Payment) TableName() string {
	return "payments"
}

