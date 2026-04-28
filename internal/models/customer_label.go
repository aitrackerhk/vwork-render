package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CustomerLabel 客戶標籤
type CustomerLabel struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant    Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name      string     `gorm:"type:varchar(255);not null" json:"name"`
	Color     string     `gorm:"type:varchar(7);not null;default:'#007bff'" json:"color"`
	Status    string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	TrashedAt *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"`
}

func (CustomerLabel) TableName() string {
	return "customer_labels"
}

// BeforeCreate 創建前設置 UUID
func (cl *CustomerLabel) BeforeCreate(tx *gorm.DB) error {
	if cl.ID == uuid.Nil {
		cl.ID = uuid.New()
	}
	return nil
}

// CustomerLabelRelation 客戶標籤關聯（多對多）
type CustomerLabelRelation struct {
	ID         uuid.UUID     `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CustomerID uuid.UUID     `gorm:"type:uuid;not null;index" json:"customer_id"`
	Customer   Customer      `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	LabelID    uuid.UUID     `gorm:"type:uuid;not null;index" json:"label_id"`
	Label      CustomerLabel `gorm:"foreignKey:LabelID" json:"label,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
}

func (CustomerLabelRelation) TableName() string {
	return "customer_label_relations"
}

// BeforeCreate 創建前設置 UUID
func (clr *CustomerLabelRelation) BeforeCreate(tx *gorm.DB) error {
	if clr.ID == uuid.Nil {
		clr.ID = uuid.New()
	}
	return nil
}
