package models

import (
	"time"

	"github.com/google/uuid"
)

type Supplier struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Code             string     `gorm:"type:varchar(100)" json:"code"`
	Name             string     `gorm:"type:varchar(255);not null" json:"name"`
	LastName         string     `gorm:"type:varchar(255)" json:"last_name"`
	Email            string     `gorm:"type:varchar(255)" json:"email"`
	Phone            string     `gorm:"type:varchar(50)" json:"phone"`
	PhoneCountryCode string     `gorm:"type:varchar(10)" json:"phone_country_code"`
	Address          string     `gorm:"type:text" json:"address"`
	Status           string     `gorm:"type:varchar(50);not null;default:'active'" json:"status"`
	ExtraFields      JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	CreatedBy        *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy        *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	TrashedAt        *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Supplier) TableName() string {
	return "suppliers"
}
