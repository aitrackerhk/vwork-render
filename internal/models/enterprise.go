package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Enterprise 企業
type Enterprise struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_tenant_enterprise" json:"tenant_id"`
	Tenant     Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name       string    `gorm:"type:varchar(255);not null" json:"name"`
	Code       *string   `gorm:"type:varchar(50)" json:"code,omitempty"`
	Domain     *string   `gorm:"type:varchar(255)" json:"domain,omitempty"`
	LogoURL    *string   `gorm:"column:logo_url;type:varchar(500)" json:"logo_url,omitempty"`
	Address    *string   `gorm:"type:text" json:"address,omitempty"`
	// Phone 為前端企業設定頁使用的欄位；實際存放於 extra_fields.phone（不新增資料表欄位）
	Phone *string `gorm:"-" json:"phone,omitempty"`
	// Email 為前端企業設定頁使用的欄位；實際存放於 extra_fields.email（不新增資料表欄位）
	Email *string `gorm:"-" json:"email,omitempty"`
	Timezone   string    `gorm:"type:varchar(50);default:'Asia/Hong_Kong'" json:"timezone"`
	Status     string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB    `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Enterprise) TableName() string {
	return "enterprises"
}

// BeforeCreate 創建前設置
func (e *Enterprise) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.ExtraFields == nil {
		e.ExtraFields = make(JSONB)
	}
	return nil
}

