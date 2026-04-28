package models

import (
	"time"

	"github.com/google/uuid"
)

// Draft 草稿
// 用於保存動態表單的草稿資料（每個用戶獨立）
type Draft struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	PageName      string    `gorm:"type:varchar(100);not null;index" json:"page_name"`
	KeyFieldValue string    `gorm:"type:varchar(255)" json:"key_field_value,omitempty"`
	Data          JSONB     `gorm:"type:jsonb;default:'{}'" json:"data"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (Draft) TableName() string {
	return "drafts"
}
