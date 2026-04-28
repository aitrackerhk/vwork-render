package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ActivityLog 活動記錄
type ActivityLog struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant      Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	User        User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Action      string    `gorm:"type:varchar(50);not null;index" json:"action"` // create, update, delete, login, logout, setting_change
	ResourceType string   `gorm:"type:varchar(100);not null;index" json:"resource_type"` // user, customer, product, order, setting, etc.
	ResourceID   *uuid.UUID `gorm:"type:uuid;index" json:"resource_id,omitempty"` // 資源 ID（如果有的話）
	Description string    `gorm:"type:text;not null" json:"description"` // 描述
	Changes     JSONB     `gorm:"type:jsonb;default:'{}'" json:"changes,omitempty"` // 變更的詳細資料（舊值、新值）
	IPAddress   string    `gorm:"type:varchar(50)" json:"ip_address,omitempty"` // IP 地址
	UserAgent   string    `gorm:"type:text" json:"user_agent,omitempty"` // User Agent
	CreatedAt   time.Time `json:"created_at"`
}

// BeforeCreate 創建前設置 UUID
func (a *ActivityLog) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.Changes == nil {
		a.Changes = make(JSONB)
	}
	return nil
}

// TableName 指定表名
func (ActivityLog) TableName() string {
	return "activity_logs"
}

