package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserMenuSettings 用戶菜單設定表
type UserMenuSettings struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID   uuid.UUID `gorm:"type:uuid;not null;index:idx_user_menu_settings_tenant_user" json:"tenant_id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index:idx_user_menu_settings_tenant_user" json:"user_id"`
	MenuConfig JSONB     `gorm:"type:jsonb;default:'[]'" json:"menu_config"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 設置表名
func (UserMenuSettings) TableName() string {
	return "user_menu_settings"
}

// BeforeCreate 創建前設置 UUID
func (u *UserMenuSettings) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	if u.MenuConfig == nil {
		u.MenuConfig = make(JSONB)
	}
	return nil
}

// MenuItemConfig 單個菜單項的設定
type MenuItemConfig struct {
	Key     string `json:"key"`     // 菜單項唯一標識
	Visible bool   `json:"visible"` // 是否顯示
	Order   int    `json:"order"`   // 排序順序
}
