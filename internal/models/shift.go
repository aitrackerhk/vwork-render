package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Shift 工作時段
type Shift struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name      string     `gorm:"type:varchar(255);not null" json:"name"` // 時段名稱
	StartTime SQLTime    `gorm:"type:time;not null" json:"start_time"`   // 上班時間
	EndTime   SQLTime    `gorm:"type:time;not null" json:"end_time"`     // 下班時間
	IsDefault bool       `gorm:"default:false" json:"is_default"`        // 是否為預設時段
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	TrashedAt *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間

	// Relations
	Tenant Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Users  []User `gorm:"foreignKey:ShiftID" json:"users,omitempty"`
}

func (Shift) TableName() string {
	return "shifts"
}

func (s *Shift) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}
