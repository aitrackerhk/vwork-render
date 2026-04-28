package models

import (
	"time"

	"github.com/google/uuid"
)

// PayrollAdjustmentPreset 薪資附加項目 preset（常用模板）
type PayrollAdjustmentPreset struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"`
	Direction   string    `gorm:"type:varchar(20);not null;default:'add'" json:"direction"` // add / subtract
	Mode        string    `gorm:"type:varchar(20);not null;default:'fixed'" json:"mode"`   // fixed / percent
	RatePercent float64   `gorm:"not null;default:0" json:"rate_percent"`                  // percent number (e.g. 5 = 5%)
	Amount      float64   `gorm:"not null;default:0" json:"amount"`                        // fixed amount
	Status      string    `gorm:"type:varchar(20);not null;default:'active'" json:"status"` // active / inactive
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (PayrollAdjustmentPreset) TableName() string {
	return "payroll_adjustment_presets"
}


