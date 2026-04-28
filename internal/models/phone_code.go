package models

import (
	"time"

	"github.com/google/uuid"
)

type PhoneCountryCode struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_phone_code_tenant_code" json:"tenant_id"`
	Code      string     `gorm:"type:varchar(10);not null;uniqueIndex:idx_phone_code_tenant_code" json:"code"`
	Name      string     `gorm:"type:varchar(100);not null" json:"name"`
	IsDefault bool       `gorm:"default:false" json:"is_default"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	TrashedAt *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (PhoneCountryCode) TableName() string {
	return "phone_country_codes"
}
