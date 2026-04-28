package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PageViewDaily 每租戶每頁每日瀏覽量
// 注意：本專案多數 migration 使用 SQL 檔；此 model 主要供查詢/Scan 使用。
type PageViewDaily struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	PageID    uuid.UUID `gorm:"type:uuid;not null" json:"page_id"`
	ViewDate  time.Time `gorm:"type:date;not null" json:"view_date"`
	ViewCount int       `gorm:"not null;default:0" json:"view_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (PageViewDaily) TableName() string {
	return "page_view_daily"
}

func (pvd *PageViewDaily) BeforeCreate(tx *gorm.DB) error {
	if pvd.ID == uuid.Nil {
		pvd.ID = uuid.New()
	}
	return nil
}


