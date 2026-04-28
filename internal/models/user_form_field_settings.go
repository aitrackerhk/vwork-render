package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserFormFieldSettings 租戶表單欄位設定表（租戶級別，所有用戶共享）
type UserFormFieldSettings struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_form_field_settings_tenant_page" json:"tenant_id"`
	PageName    string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_user_form_field_settings_tenant_page" json:"page_name"`
	FieldConfig JSONB     `gorm:"type:jsonb;default:'{\"fields\": [], \"extraFields\": []}'" json:"field_config"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 設置表名
func (UserFormFieldSettings) TableName() string {
	return "user_form_field_settings"
}

// BeforeCreate 創建前設置 UUID
func (u *UserFormFieldSettings) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	if u.FieldConfig == nil {
		u.FieldConfig = JSONB{
			"fields":      []interface{}{},
			"extraFields": []interface{}{},
		}
	}
	return nil
}

// FieldSettingItem 單個欄位的設定
type FieldSettingItem struct {
	Key     string `json:"key"`     // 欄位唯一標識
	Visible bool   `json:"visible"` // 是否顯示
	Order   int    `json:"order"`   // 排序順序
}

// ExtraFieldItem 額外欄位定義
type ExtraFieldItem struct {
	Key   string `json:"key"`   // 欄位唯一標識
	Label string `json:"label"` // 欄位標籤
	Type  string `json:"type"`  // 欄位類型：text, number, date, textarea, email
}
