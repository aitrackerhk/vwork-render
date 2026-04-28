package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Product 產品模型
type Product struct {
	ID                      uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID                uuid.UUID      `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Code                    string         `gorm:"type:varchar(100)" json:"code"`
	SKU                     string         `gorm:"type:varchar(100);index" json:"sku"`
	Barcode                 string         `gorm:"type:varchar(100);index" json:"barcode"`
	Name                    string         `gorm:"type:varchar(255);not null" json:"name"`
	Description             string         `gorm:"type:text" json:"description"`
	Category                string         `gorm:"type:varchar(100)" json:"category"`
	SubstanceCategory       string         `gorm:"type:varchar(100);column:substance_category" json:"substance_category"`
	ImageURL                string         `gorm:"type:varchar(500);column:image_url" json:"image_url,omitempty"`
	Price                   float64        `gorm:"type:decimal(15,2)" json:"price"`
	Cost                    float64        `gorm:"type:decimal(15,2)" json:"cost"`
	StockQuantity           int            `gorm:"default:0" json:"stock_quantity"`
	Unit                    string         `gorm:"type:varchar(50)" json:"unit"`
	Weight                  float64        `gorm:"type:decimal(10,2);default:0" json:"weight"`                                              // 重量（公斤）
	Area                    float64        `gorm:"type:decimal(10,2);default:0" json:"area"`                                                // 面積（平方米）
	IsServicePackage        bool           `gorm:"type:boolean;default:false;column:is_service_package" json:"is_service_package"`          // 是否為服務套票
	ServicePackageServiceID *uuid.UUID     `gorm:"type:uuid;column:service_package_service_id" json:"service_package_service_id,omitempty"` // 服務套票對應的服務ID
	ServicePackageService   *Service       `gorm:"foreignKey:ServicePackageServiceID" json:"service_package_service,omitempty"`
	IsNonInventory          bool           `gorm:"type:boolean;default:false;column:is_non_inventory" json:"is_non_inventory"` // 非庫存類產品，若為 true 則不計算庫存和配送
	Status                  string         `gorm:"type:varchar(50);not null;default:'active'" json:"status"`
	ProductTypeID           *uuid.UUID     `gorm:"type:uuid" json:"product_type_id,omitempty"`
	ProductType             *ProductType   `gorm:"foreignKey:ProductTypeID" json:"product_type,omitempty"`
	BrandID                 *uuid.UUID     `gorm:"type:uuid" json:"brand_id,omitempty"`
	Brand                   *Brand         `gorm:"foreignKey:BrandID" json:"brand,omitempty"`
	DefaultSupplierID       *uuid.UUID     `gorm:"type:uuid;column:default_supplier_id" json:"default_supplier_id,omitempty"`
	DefaultSupplier         *Supplier      `gorm:"foreignKey:DefaultSupplierID" json:"default_supplier,omitempty"`
	DefaultWarehouseID      *uuid.UUID     `gorm:"type:uuid;column:default_warehouse_id" json:"default_warehouse_id,omitempty"`
	DefaultWarehouse        *Warehouse     `gorm:"foreignKey:DefaultWarehouseID" json:"default_warehouse,omitempty"`
	DefaultWarehouseZoneID  *uuid.UUID     `gorm:"type:uuid;column:default_warehouse_zone_id" json:"default_warehouse_zone_id,omitempty"`
	DefaultWarehouseZone    *WarehouseZone `gorm:"foreignKey:DefaultWarehouseZoneID" json:"default_warehouse_zone,omitempty"`
	AllowBackorder          bool           `gorm:"type:boolean;default:false;column:allow_backorder" json:"allow_backorder"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
	CreatedBy               *uuid.UUID     `gorm:"type:uuid" json:"created_by"`
	UpdatedBy               *uuid.UUID     `gorm:"type:uuid" json:"updated_by"`
	TrashedAt               *time.Time     `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields             JSONB          `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`

	// 關聯：產品稅
	ProductTaxes []ProductTax `gorm:"many2many:product_tax_relations;foreignKey:ID;joinForeignKey:ProductID;References:ID;joinReferences:TaxID" json:"product_taxes,omitempty"`
	// 關聯：產品標籤
	Labels []ProductLabel `gorm:"many2many:product_label_relations;foreignKey:ID;joinForeignKey:ProductID;References:ID;joinReferences:LabelID" json:"labels,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (p *Product) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (Product) TableName() string {
	return "products"
}
