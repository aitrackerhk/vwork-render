package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProductLabel 產品標籤
type ProductLabel struct {
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

func (ProductLabel) TableName() string {
	return "product_labels"
}

// BeforeCreate 創建前設置 UUID
func (pl *ProductLabel) BeforeCreate(tx *gorm.DB) error {
	if pl.ID == uuid.Nil {
		pl.ID = uuid.New()
	}
	return nil
}

// ProductLabelRelation 產品標籤關聯（多對多）
type ProductLabelRelation struct {
	ID        uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	ProductID uuid.UUID    `gorm:"type:uuid;not null;index" json:"product_id"`
	Product   Product      `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	LabelID   uuid.UUID    `gorm:"type:uuid;not null;index" json:"label_id"`
	Label     ProductLabel `gorm:"foreignKey:LabelID" json:"label,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}

func (ProductLabelRelation) TableName() string {
	return "product_label_relations"
}

// BeforeCreate 創建前設置 UUID
func (plr *ProductLabelRelation) BeforeCreate(tx *gorm.DB) error {
	if plr.ID == uuid.Nil {
		plr.ID = uuid.New()
	}
	return nil
}
