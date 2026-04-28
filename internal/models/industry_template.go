package models

import (
	"time"

	"github.com/google/uuid"
)

// IndustryTemplate 行業模板
type IndustryTemplate struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Code           string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"`
	Name           string    `gorm:"type:varchar(255);not null" json:"name"`
	NameEn         *string   `gorm:"type:varchar(255)" json:"name_en,omitempty"`
	Description    *string   `gorm:"type:text" json:"description,omitempty"`
	DescriptionEn  *string   `gorm:"type:text" json:"description_en,omitempty"`
	EnabledModules JSONB     `gorm:"type:jsonb;default:'[]'" json:"enabled_modules"`
	DefaultFields  JSONB     `gorm:"type:jsonb;default:'{}'" json:"default_fields"`
	Icon           *string   `gorm:"type:varchar(100)" json:"icon,omitempty"`
	IsActive       bool      `gorm:"default:true" json:"is_active"`
	SortOrder      int       `gorm:"default:0" json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (IndustryTemplate) TableName() string {
	return "industry_templates"
}

