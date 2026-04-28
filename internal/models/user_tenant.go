package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserTenant 使用者與租戶關聯
// 允許一個使用者加入多個租戶
// role 可選，用於儲存租戶內角色或權限描述
// employee_number 每個租戶獨立的員工編號
// is_default/last_used_at 用於記錄最後選擇的租戶
//
// 注意：租戶切換時會更新 last_used_at 並同步 users.tenant_id 作為預設租戶
// 以保持既有邏輯與 middleware 行為一致。
type UserTenant struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	User           *User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant         *Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Role           string     `gorm:"type:varchar(50)" json:"role,omitempty"`
	EmployeeNumber string     `gorm:"type:varchar(100)" json:"employee_number,omitempty"`
	IsDefault      bool       `gorm:"default:false" json:"is_default"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (u *UserTenant) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

func (UserTenant) TableName() string {
	return "user_tenants"
}
