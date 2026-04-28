package models

import (
	"time"

	"github.com/google/uuid"
)

// Currency 貨幣
type Currency struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_currency_tenant_code" json:"tenant_id"`
	Code         string     `gorm:"type:varchar(10);not null;uniqueIndex:idx_currency_tenant_code" json:"code"`
	Name         string     `gorm:"type:varchar(100);not null" json:"name"`
	Symbol       *string    `gorm:"type:varchar(10)" json:"symbol,omitempty"`
	ExchangeRate float64    `gorm:"type:decimal(18,6);default:1.0" json:"exchange_rate"`
	IsDefault    bool       `gorm:"default:false" json:"is_default"` // 是否為系統預設
	Status       string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields  JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	TrashedAt    *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Currency) TableName() string {
	return "currencies"
}
