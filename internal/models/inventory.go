package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Inventory 庫存模型
type Inventory struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ProductID       uuid.UUID `gorm:"type:uuid;not null;index" json:"product_id"`
	WarehouseLocation string  `gorm:"type:varchar(255)" json:"warehouse_location"`
	Quantity        int      `gorm:"default:0" json:"quantity"`
	ReservedQuantity int     `gorm:"default:0" json:"reserved_quantity"`
	LastUpdatedAt   time.Time `json:"last_updated_at"`
	ExtraFields     JSONB    `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	
	// 關聯
	Product         *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (i *Inventory) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (Inventory) TableName() string {
	return "inventory"
}

