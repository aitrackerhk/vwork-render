package models

import (
	"time"

	"github.com/google/uuid"
)

// Department 部門
type Department struct {
	ID        uuid.UUID   `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CompanyID uuid.UUID   `gorm:"type:uuid;not null" json:"company_id"`
	Company   Company     `gorm:"foreignKey:CompanyID" json:"company,omitempty"`
	Name      string      `gorm:"type:varchar(255);not null" json:"name"`
	Code      *string     `gorm:"type:varchar(50)" json:"code,omitempty"`
	ParentID  *uuid.UUID  `gorm:"type:uuid" json:"parent_id,omitempty"`
	Parent    *Department `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	// JSONB array of strings, e.g. ["dashboard","customers"]
	Permissions StringArrayJSONB `gorm:"type:jsonb;default:'[]'" json:"permissions,omitempty"`
	Status      string           `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB            `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	TrashedAt   *time.Time       `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Department) TableName() string {
	return "departments"
}
