package models

import (
	"time"

	"github.com/google/uuid"
)

// TenantModule 租戶模塊配置
type TenantModule struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID   uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant     Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	ModuleCode string    `gorm:"type:varchar(50);not null" json:"module_code"`
	IsEnabled  bool      `gorm:"default:true" json:"is_enabled"`
	Config     JSONB     `gorm:"type:jsonb;default:'{}'" json:"config"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (TenantModule) TableName() string {
	return "tenant_modules"
}

