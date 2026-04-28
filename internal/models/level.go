package models

import (
	"time"

	"github.com/google/uuid"
)

// Level 級別
type Level struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	DepartmentID uuid.UUID  `gorm:"type:uuid;not null" json:"department_id"`
	Department   Department `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
	Name         string     `gorm:"type:varchar(255);not null" json:"name"`
	Code         *string    `gorm:"type:varchar(50)" json:"code,omitempty"`
	LevelOrder   int        `gorm:"default:0" json:"level_order"`
	// JSONB array of strings, e.g. ["dashboard","customers"]
	Permissions StringArrayJSONB `gorm:"type:jsonb;default:'[]'" json:"permissions,omitempty"`
	Status      string           `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB            `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	TrashedAt   *time.Time       `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Level) TableName() string {
	return "levels"
}
