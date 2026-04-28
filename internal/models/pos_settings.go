package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PosSettings POS 設定
type PosSettings struct {
	ID                  uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID            uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	DepositPercent      float64   `gorm:"type:decimal(5,2);default:0" json:"deposit_percent"`       // 訂金百分比
	DepositFixed        float64   `gorm:"type:decimal(10,2);default:0" json:"deposit_fixed"`        // 訂金固定金額
	AllowGuestCheckout  bool      `gorm:"type:bool;default:false" json:"allow_guest_checkout"`      // 允許訪客結帳
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// BeforeCreate 創建前設置 UUID
func (ps *PosSettings) BeforeCreate(tx *gorm.DB) error {
	if ps.ID == uuid.Nil {
		ps.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (PosSettings) TableName() string {
	return "pos_settings"
}

