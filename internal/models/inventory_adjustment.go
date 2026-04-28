package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InventoryAdjustment 庫存調整記錄
type InventoryAdjustment struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ProductID       uuid.UUID `gorm:"type:uuid;not null;index" json:"product_id"`
	AdjustmentType  string    `gorm:"type:varchar(50);not null" json:"adjustment_type"` // increase, decrease, set
	Quantity        int       `gorm:"not null" json:"quantity"`
	PreviousQuantity int      `gorm:"not null" json:"previous_quantity"`
	NewQuantity     int       `gorm:"not null" json:"new_quantity"`
	Reason          string    `gorm:"type:varchar(255)" json:"reason"`
	Notes           string    `gorm:"type:text" json:"notes"`
	WarehouseLocation string  `gorm:"type:varchar(255)" json:"warehouse_location"`
	WarehouseID      *uuid.UUID `gorm:"type:uuid" json:"warehouse_id"`
	CreatedBy       *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	
	// 關聯
	Product         *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Warehouse       *Warehouse `gorm:"foreignKey:WarehouseID" json:"warehouse,omitempty"`
	User            *User     `gorm:"foreignKey:CreatedBy" json:"user,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (ia *InventoryAdjustment) BeforeCreate(tx *gorm.DB) error {
	if ia.ID == uuid.Nil {
		ia.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (InventoryAdjustment) TableName() string {
	return "inventory_adjustments"
}

