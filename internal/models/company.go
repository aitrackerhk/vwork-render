package models

import (
	"time"

	"github.com/google/uuid"
)

// Company 公司
type Company struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	EnterpriseID uuid.UUID  `gorm:"type:uuid;not null" json:"enterprise_id"`
	Enterprise   Enterprise `gorm:"foreignKey:EnterpriseID" json:"enterprise,omitempty"`
	Name         string     `gorm:"type:varchar(255);not null" json:"name"`
	Code         *string    `gorm:"type:varchar(50)" json:"code,omitempty"`
	Status       string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields  JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	TrashedAt    *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Company) TableName() string {
	return "companies"
}
