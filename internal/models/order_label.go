package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrderLabel 訂單標籤
type OrderLabel struct {
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

func (OrderLabel) TableName() string {
	return "order_labels"
}

// BeforeCreate 創建前設置 UUID
func (ol *OrderLabel) BeforeCreate(tx *gorm.DB) error {
	if ol.ID == uuid.Nil {
		ol.ID = uuid.New()
	}
	return nil
}

// OrderLabelRelation 訂單標籤關聯（多對多）
type OrderLabelRelation struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OrderID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"order_id"`
	Order     Order      `gorm:"foreignKey:OrderID" json:"order,omitempty"`
	LabelID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"label_id"`
	Label     OrderLabel `gorm:"foreignKey:LabelID" json:"label,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (OrderLabelRelation) TableName() string {
	return "order_label_relations"
}

// BeforeCreate 創建前設置 UUID
func (olr *OrderLabelRelation) BeforeCreate(tx *gorm.DB) error {
	if olr.ID == uuid.Nil {
		olr.ID = uuid.New()
	}
	return nil
}
