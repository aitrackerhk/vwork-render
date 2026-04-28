package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InventoryCount 庫存盤點記錄
type InventoryCount struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	CountNumber     string    `gorm:"type:varchar(100);not null" json:"count_number"`
	CountDate       time.Time `gorm:"type:date;not null" json:"count_date"`
	WarehouseLocation string  `gorm:"type:varchar(255)" json:"warehouse_location"`
	WarehouseID     *uuid.UUID `gorm:"type:uuid" json:"warehouse_id"`
	Status          string    `gorm:"type:varchar(50);not null;default:'draft'" json:"status"` // draft, in_progress, completed, cancelled
	Notes           string    `gorm:"type:text" json:"notes"`
	CreatedBy       *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	
	// 關聯
	User            *User              `gorm:"foreignKey:CreatedBy" json:"user,omitempty"`
	Warehouse       *Warehouse         `gorm:"foreignKey:WarehouseID" json:"warehouse,omitempty"`
	CountItems      []InventoryCountItem `gorm:"foreignKey:CountID" json:"count_items,omitempty"`
}

// InventoryCountItem 盤點明細
type InventoryCountItem struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	CountID         uuid.UUID `gorm:"type:uuid;not null;index" json:"count_id"`
	ProductID       uuid.UUID `gorm:"type:uuid;not null;index" json:"product_id"`
	SystemQuantity  int       `gorm:"not null" json:"system_quantity"` // 系統庫存
	CountedQuantity int       `gorm:"not null" json:"counted_quantity"` // 盤點數量
	Variance        int       `gorm:"not null" json:"variance"` // 差異
	Notes           string    `gorm:"type:text" json:"notes"`
	CreatedAt       time.Time `json:"created_at"`
	
	// 關聯
	Product         *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (ic *InventoryCount) BeforeCreate(tx *gorm.DB) error {
	if ic.ID == uuid.Nil {
		ic.ID = uuid.New()
	}
	return nil
}

func (ici *InventoryCountItem) BeforeCreate(tx *gorm.DB) error {
	if ici.ID == uuid.Nil {
		ici.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (InventoryCount) TableName() string {
	return "inventory_counts"
}

func (InventoryCountItem) TableName() string {
	return "inventory_count_items"
}

