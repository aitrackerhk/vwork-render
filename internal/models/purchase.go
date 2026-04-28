package models

import (
	"time"

	"github.com/google/uuid"
)

// PurchaseOrder 採購單
type PurchaseOrder struct {
	ID                   uuid.UUID            `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID             uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex:idx_purchase_orders_tenant_order_number" json:"tenant_id"`
	Tenant               Tenant               `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	SupplierID           *uuid.UUID           `gorm:"type:uuid" json:"supplier_id,omitempty"`
	Supplier             *Supplier            `gorm:"foreignKey:SupplierID" json:"supplier,omitempty"`
	OrderNumber          string               `gorm:"type:varchar(100);uniqueIndex:idx_purchase_orders_tenant_order_number" json:"order_number"`
	OrderDate            time.Time            `gorm:"type:date;not null" json:"order_date"`
	ExpectedDeliveryDate *time.Time           `gorm:"type:date" json:"expected_delivery_date,omitempty"`
	TotalAmount          float64              `gorm:"type:decimal(18,2);default:0.00" json:"total_amount"`
	DiscountAmount       float64              `gorm:"type:decimal(18,2);default:0.00" json:"discount_amount"`
	TaxAmount            float64              `gorm:"type:decimal(18,2);default:0.00" json:"tax_amount"`
	FinalAmount          float64              `gorm:"type:decimal(18,2);default:0.00" json:"final_amount"`
	Status               string               `gorm:"type:varchar(50);default:'draft'" json:"status"`
	Notes                string               `gorm:"type:text" json:"notes"`
	PurchaseOrderItems   []PurchaseOrderItem  `gorm:"foreignKey:PurchaseOrderID" json:"purchase_items,omitempty"`
	Labels               []PurchaseOrderLabel `gorm:"many2many:purchase_order_label_relations;foreignKey:ID;joinForeignKey:PurchaseOrderID;References:ID;joinReferences:LabelID" json:"labels,omitempty"`
	ExtraFields          JSONB                `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
	TrashedAt            *time.Time           `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (PurchaseOrder) TableName() string {
	return "purchase_orders"
}

// PurchaseOrderItem 採購單明細
type PurchaseOrderItem struct {
	ID               uuid.UUID     `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	PurchaseOrderID  uuid.UUID     `gorm:"type:uuid;not null" json:"purchase_order_id"`
	PurchaseOrder    PurchaseOrder `gorm:"foreignKey:PurchaseOrderID" json:"purchase_order,omitempty"`
	ProductID        *uuid.UUID    `gorm:"type:uuid" json:"product_id,omitempty"`
	Product          *Product      `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Quantity         int           `gorm:"not null" json:"quantity"`
	UnitPrice        float64       `gorm:"type:decimal(18,2);default:0.00" json:"unit_price"`
	DiscountAmount   float64       `gorm:"type:decimal(18,2);default:0.00" json:"discount_amount"`
	TaxRate          float64       `gorm:"type:decimal(5,2);default:0.00" json:"tax_rate"`
	TotalAmount      float64       `gorm:"type:decimal(18,2);default:0.00" json:"total_amount"`
	ReceivedQuantity int           `gorm:"default:0" json:"received_quantity"`
	Notes            string        `gorm:"type:text" json:"notes"`
	ExtraFields      JSONB         `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
	TrashedAt        *time.Time    `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (PurchaseOrderItem) TableName() string {
	return "purchase_order_items"
}
