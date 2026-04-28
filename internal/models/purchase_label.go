package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PurchaseOrderLabel 採購標籤
type PurchaseOrderLabel struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant    Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name      string     `gorm:"type:varchar(255);not null" json:"name"`
	Color     string     `gorm:"type:varchar(7);not null;default:'#007bff'" json:"color"`
	Status    string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	TrashedAt *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (PurchaseOrderLabel) TableName() string {
	return "purchase_order_labels"
}

// BeforeCreate 創建前設置 UUID
func (pol *PurchaseOrderLabel) BeforeCreate(tx *gorm.DB) error {
	if pol.ID == uuid.Nil {
		pol.ID = uuid.New()
	}
	return nil
}

// PurchaseOrderLabelRelation 採購標籤關聯（多對多）
type PurchaseOrderLabelRelation struct {
	ID              uuid.UUID          `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	PurchaseOrderID uuid.UUID          `gorm:"type:uuid;not null;index" json:"purchase_order_id"`
	PurchaseOrder   PurchaseOrder      `gorm:"foreignKey:PurchaseOrderID" json:"purchase_order,omitempty"`
	LabelID         uuid.UUID          `gorm:"type:uuid;not null;index" json:"label_id"`
	Label           PurchaseOrderLabel `gorm:"foreignKey:LabelID" json:"label,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
}

func (PurchaseOrderLabelRelation) TableName() string {
	return "purchase_order_label_relations"
}

// BeforeCreate 創建前設置 UUID
func (polr *PurchaseOrderLabelRelation) BeforeCreate(tx *gorm.DB) error {
	if polr.ID == uuid.Nil {
		polr.ID = uuid.New()
	}
	return nil
}
