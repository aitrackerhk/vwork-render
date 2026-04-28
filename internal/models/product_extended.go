package models

import (
	"time"

	"github.com/google/uuid"
)

// ProductType 產品類型
type ProductType struct {
	ID          uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID    `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant      Tenant       `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name        string       `gorm:"type:varchar(255);not null" json:"name"`
	Code        *string      `gorm:"type:varchar(50)" json:"code,omitempty"`
	ImageURL    *string      `gorm:"type:varchar(500)" json:"image_url,omitempty"`
	ParentID    *uuid.UUID   `gorm:"type:uuid" json:"parent_id,omitempty"`
	Parent      *ProductType `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Status      string       `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB        `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	TrashedAt   *time.Time   `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (ProductType) TableName() string {
	return "product_types"
}

// ProductAttribute 產品屬性
type ProductAttribute struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant        Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name          string    `gorm:"type:varchar(255);not null" json:"name"`
	Code          *string   `gorm:"type:varchar(50)" json:"code,omitempty"`
	AttributeType string    `gorm:"type:varchar(50);not null" json:"attribute_type"`
	Options       JSONB     `gorm:"type:jsonb" json:"options,omitempty"`
	IsRequired    bool      `gorm:"default:false" json:"is_required"`
	Status        string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields   JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (ProductAttribute) TableName() string {
	return "product_attributes"
}

// ProductAttributeValue 產品屬性值
type ProductAttributeValue struct {
	ID          uuid.UUID        `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	ProductID   uuid.UUID        `gorm:"type:uuid;not null" json:"product_id"`
	Product     Product          `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	AttributeID uuid.UUID        `gorm:"type:uuid;not null" json:"attribute_id"`
	Attribute   ProductAttribute `gorm:"foreignKey:AttributeID" json:"attribute,omitempty"`
	Value       string           `gorm:"type:text" json:"value"`
	Status      string           `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

func (ProductAttributeValue) TableName() string {
	return "product_attribute_values"
}

// Brand 品牌
type Brand struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant      Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"`
	Code        *string   `gorm:"type:varchar(50)" json:"code,omitempty"`
	LogoURL     *string   `gorm:"type:varchar(500)" json:"logo_url,omitempty"`
	Description string    `gorm:"type:text" json:"description"`
	Status      string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Brand) TableName() string {
	return "brands"
}
