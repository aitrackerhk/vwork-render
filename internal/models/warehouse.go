package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Warehouse 倉庫
type Warehouse struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant           Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name             string     `gorm:"type:varchar(255);not null" json:"name"`
	Code             string     `gorm:"type:varchar(100);not null" json:"code"`
	Address          string     `gorm:"type:text" json:"address"`
	ContactPerson    string     `gorm:"type:varchar(255)" json:"contact_person"`
	PhoneCountryCode string     `gorm:"type:varchar(10)" json:"phone_country_code"`
	Phone            string     `gorm:"type:varchar(50)" json:"phone"`
	Email            string     `gorm:"type:varchar(255)" json:"email"`
	Status           string     `gorm:"type:varchar(50);default:'active'" json:"status"`
	IsDefault        bool       `gorm:"default:false" json:"is_default"` // 是否為系統預設倉庫
	ExtraFields      JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CreatedBy        *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy        *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	TrashedAt        *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

// BeforeCreate 創建前設置 UUID
func (w *Warehouse) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (Warehouse) TableName() string {
	return "warehouses"
}

// WarehouseZone 倉庫區
type WarehouseZone struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant      Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	WarehouseID uuid.UUID  `gorm:"type:uuid;not null;index" json:"warehouse_id"`
	Warehouse   *Warehouse `gorm:"foreignKey:WarehouseID" json:"warehouse,omitempty"`
	Name        string     `gorm:"type:varchar(255);not null" json:"name"`
	Code        string     `gorm:"type:varchar(100)" json:"code"`
	Description string     `gorm:"type:text" json:"description"`
	IsDefault   bool       `gorm:"default:false" json:"is_default"` // 是否為該倉庫的預設區
	Status      string     `gorm:"type:varchar(50);default:'active'" json:"status"`
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CreatedBy   *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy   *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	TrashedAt   *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (wz *WarehouseZone) BeforeCreate(tx *gorm.DB) error {
	if wz.ID == uuid.Nil {
		wz.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (WarehouseZone) TableName() string {
	return "warehouse_zones"
}

// ProductWarehouseStock 產品倉庫庫存
type ProductWarehouseStock struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ProductID        uuid.UUID `gorm:"type:uuid;not null;index" json:"product_id"`
	WarehouseID      uuid.UUID `gorm:"type:uuid;not null;index" json:"warehouse_id"`
	Quantity         int       `gorm:"default:0" json:"quantity"`
	ReservedQuantity int       `gorm:"default:0" json:"reserved_quantity"`
	LastUpdatedAt    time.Time `json:"last_updated_at"`
	ExtraFields      JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`

	// 關聯
	Product   *Product   `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Warehouse *Warehouse `gorm:"foreignKey:WarehouseID" json:"warehouse,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (pws *ProductWarehouseStock) BeforeCreate(tx *gorm.DB) error {
	if pws.ID == uuid.Nil {
		pws.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (ProductWarehouseStock) TableName() string {
	return "product_warehouse_stocks"
}
