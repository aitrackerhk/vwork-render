package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProductTax 產品稅設定
type ProductTax struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name     string    `gorm:"type:varchar(255);not null" json:"name"`
	Code     *string   `gorm:"type:varchar(50)" json:"code,omitempty"`
	TaxMode  string    `gorm:"type:varchar(20);not null;default:'percent'" json:"tax_mode"` // percent / fixed
	TaxValue float64   `gorm:"type:decimal(15,4);not null;default:0" json:"tax_value"`
	// DefaultInclude: jsonb array，例如 ["order"]（產品稅只應用於訂單）
	DefaultInclude StringArrayJSONB `gorm:"type:jsonb;default:'[]'" json:"default_include,omitempty"`
	Status         string           `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields    JSONB            `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	TrashedAt      *time.Time       `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (ProductTax) TableName() string { return "product_taxes" }

func (t *ProductTax) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// ServiceTax 服務稅設定
type ServiceTax struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name     string    `gorm:"type:varchar(255);not null" json:"name"`
	Code     *string   `gorm:"type:varchar(50)" json:"code,omitempty"`
	TaxMode  string    `gorm:"type:varchar(20);not null;default:'percent'" json:"tax_mode"` // percent / fixed
	TaxValue float64   `gorm:"type:decimal(15,4);not null;default:0" json:"tax_value"`
	// DefaultInclude: jsonb array，例如 ["service_order"]（服務稅只應用於服務單）
	DefaultInclude StringArrayJSONB `gorm:"type:jsonb;default:'[]'" json:"default_include,omitempty"`
	Status         string           `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields    JSONB            `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	TrashedAt      *time.Time       `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (ServiceTax) TableName() string { return "service_taxes" }

func (t *ServiceTax) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// ProductTaxRelation 產品-稅關聯
type ProductTaxRelation struct {
	ProductID uuid.UUID `gorm:"type:uuid;primaryKey" json:"product_id"`
	TaxID     uuid.UUID `gorm:"type:uuid;primaryKey" json:"tax_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (ProductTaxRelation) TableName() string { return "product_tax_relations" }

// ServiceTaxRelation 服務-稅關聯
type ServiceTaxRelation struct {
	ServiceID uuid.UUID `gorm:"type:uuid;primaryKey" json:"service_id"`
	TaxID     uuid.UUID `gorm:"type:uuid;primaryKey" json:"tax_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (ServiceTaxRelation) TableName() string { return "service_tax_relations" }
