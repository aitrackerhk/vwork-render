package models

import (
	"time"

	"github.com/google/uuid"
)

// Role 角色（原級別）
type Role struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	// JSONB array of strings, e.g. ["dashboard","customers"]
	Permissions StringArrayJSONB `gorm:"type:jsonb;default:'[]'" json:"permissions,omitempty"`
	Status      string           `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	TrashedAt   *time.Time       `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Role) TableName() string {
	return "roles"
}
